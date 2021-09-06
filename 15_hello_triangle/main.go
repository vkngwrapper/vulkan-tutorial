package main

import (
	"embed"
	"github.com/CannibalVox/VKng"
	"github.com/CannibalVox/VKng/commands"
	"github.com/CannibalVox/VKng/core"
	"github.com/CannibalVox/VKng/ext_debugutils"
	"github.com/CannibalVox/VKng/ext_surface"
	"github.com/CannibalVox/VKng/ext_surface_sdl2"
	"github.com/CannibalVox/VKng/ext_swapchain"
	"github.com/CannibalVox/VKng/pipeline"
	"github.com/CannibalVox/VKng/render_pass"
	"github.com/CannibalVox/cgoalloc"
	"github.com/palantir/stacktrace"
	"github.com/veandco/go-sdl2/sdl"
	"log"
)

//go:embed shaders
var shaders embed.FS

const MaxFramesInFlight = 2

var validationLayers = []string{"VK_LAYER_KHRONOS_validation"}
var deviceExtensions = []string{ext_swapchain.ExtensionName}

const enableValidationLayers = true

type QueueFamilyIndices struct {
	GraphicsFamily *int
	PresentFamily  *int
}

func (i *QueueFamilyIndices) IsComplete() bool {
	return i.GraphicsFamily != nil && i.PresentFamily != nil
}

type SwapChainSupportDetails struct {
	Capabilities *ext_surface.Capabilities
	Formats      []ext_surface.Format
	PresentModes []ext_surface.PresentMode
}

type HelloTriangleApplication struct {
	allocator cgoalloc.Allocator
	window    *sdl.Window

	instance       *core.Instance
	debugMessenger *ext_debugutils.Messenger
	surface        *ext_surface.Surface

	physicalDevice *core.PhysicalDevice
	device         *core.Device

	graphicsQueue *core.Queue
	presentQueue  *core.Queue

	swapchain             *ext_swapchain.Swapchain
	swapchainImages       []*core.Image
	swapchainImageFormat  VKng.DataFormat
	swapchainExtent       VKng.Extent2D
	swapchainImageViews   []*core.ImageView
	swapchainFramebuffers []*render_pass.Framebuffer

	renderPass       *render_pass.RenderPass
	pipelineLayout   *pipeline.PipelineLayout
	graphicsPipeline *pipeline.Pipeline

	commandPool    *commands.CommandPool
	commandBuffers []*commands.CommandBuffer

	imageAvailableSemaphore []*core.Semaphore
	renderFinishedSemaphore []*core.Semaphore
	inFlightFence           []*core.Fence
	imagesInFlight          []*core.Fence
	currentFrame            int
}

func (app *HelloTriangleApplication) Run() error {
	err := app.initWindow()
	if err != nil {
		return err
	}

	err = app.initVulkan()
	if err != nil {
		return err
	}
	defer app.cleanup()

	return app.mainLoop()
}

func (app *HelloTriangleApplication) initWindow() error {
	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		return err
	}

	window, err := sdl.CreateWindow("Vulkan", sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED, 800, 600, sdl.WINDOW_SHOWN|sdl.WINDOW_VULKAN)
	if err != nil {
		return err
	}
	app.window = window

	return nil
}

func (app *HelloTriangleApplication) initVulkan() error {
	err := app.createInstance()
	if err != nil {
		return err
	}

	err = app.setupDebugMessenger()
	if err != nil {
		return err
	}

	err = app.createSurface()
	if err != nil {
		return err
	}

	err = app.pickPhysicalDevice()
	if err != nil {
		return err
	}

	err = app.createLogicalDevice()
	if err != nil {
		return err
	}

	err = app.createSwapchain()
	if err != nil {
		return err
	}

	err = app.createRenderPass()
	if err != nil {
		return err
	}

	err = app.createGraphicsPipeline()
	if err != nil {
		return err
	}

	err = app.createFramebuffers()
	if err != nil {
		return err
	}

	err = app.createCommandPool()
	if err != nil {
		return err
	}

	err = app.createCommandBuffers()
	if err != nil {
		return err
	}

	return app.createSyncObjects()
}

func (app *HelloTriangleApplication) mainLoop() error {
appLoop:
	for true {
		for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
			switch event.(type) {
			case *sdl.QuitEvent:
				break appLoop
			}
		}
		err := app.drawFrame()
		if err != nil {
			return err
		}
	}

	_, err := app.device.WaitForIdle()
	return err
}

func (app *HelloTriangleApplication) cleanup() {
	for _, fence := range app.inFlightFence {
		fence.Destroy()
	}

	for _, semaphore := range app.renderFinishedSemaphore {
		semaphore.Destroy()
	}

	for _, semaphore := range app.imageAvailableSemaphore {
		semaphore.Destroy()
	}

	if app.commandPool != nil {
		app.commandPool.Destroy()
	}

	for _, framebuffer := range app.swapchainFramebuffers {
		framebuffer.Destroy()
	}

	if app.graphicsPipeline != nil {
		app.graphicsPipeline.Destroy()
	}

	if app.pipelineLayout != nil {
		app.pipelineLayout.Destroy()
	}

	if app.renderPass != nil {
		app.renderPass.Destroy()
	}

	for _, imageView := range app.swapchainImageViews {
		imageView.Destroy()
	}

	if app.swapchain != nil {
		app.swapchain.Destroy()
	}

	if app.device != nil {
		app.device.Destroy()
	}

	if app.debugMessenger != nil {
		app.debugMessenger.Destroy()
	}

	if app.surface != nil {
		app.surface.Destroy()
	}

	if app.instance != nil {
		app.instance.Destroy()
	}

	if app.window != nil {
		app.window.Destroy()
	}
	sdl.Quit()

	app.allocator.Destroy()
}

func (app *HelloTriangleApplication) createInstance() error {
	instanceOptions := &core.InstanceOptions{
		ApplicationName:    "Hello Triangle",
		ApplicationVersion: VKng.CreateVersion(1, 0, 0),
		EngineName:         "No Engine",
		EngineVersion:      VKng.CreateVersion(1, 0, 0),
		VulkanVersion:      VKng.Vulkan1_2,
	}

	// Add extensions
	sdlExtensions := app.window.VulkanGetInstanceExtensions()
	extensions, _, err := core.AvailableExtensions(app.allocator)
	if err != nil {
		return err
	}

	for _, ext := range sdlExtensions {
		_, hasExt := extensions[ext]
		if !hasExt {
			return stacktrace.NewError("createinstance: cannot initialize sdl: missing extension %s", ext)
		}
		instanceOptions.ExtensionNames = append(instanceOptions.ExtensionNames, ext)
	}

	if enableValidationLayers {
		instanceOptions.ExtensionNames = append(instanceOptions.ExtensionNames, ext_debugutils.ExtensionName)
	}

	// Add layers
	layers, _, err := core.AvailableLayers(app.allocator)
	if err != nil {
		return err
	}

	if enableValidationLayers {
		for _, layer := range validationLayers {
			_, hasValidation := layers[layer]
			if !hasValidation {
				return stacktrace.NewError("createInstance: cannot add validation- layer %s not available- install LunarG Vulkan SDK", layer)
			}
			instanceOptions.LayerNames = append(instanceOptions.LayerNames, layer)
		}

		// Add debug messenger
		instanceOptions.Next = app.debugMessengerOptions()
	}

	app.instance, _, err = core.CreateInstance(app.allocator, instanceOptions)
	if err != nil {
		return err
	}

	return nil
}

func (app *HelloTriangleApplication) debugMessengerOptions() *ext_debugutils.Options {
	return &ext_debugutils.Options{
		CaptureSeverities: ext_debugutils.SeverityError | ext_debugutils.SeverityWarning,
		CaptureTypes:      ext_debugutils.TypeAll,
		Callback:          app.logDebug,
	}
}

func (app *HelloTriangleApplication) setupDebugMessenger() error {
	if !enableValidationLayers {
		return nil
	}

	var err error
	app.debugMessenger, _, err = ext_debugutils.CreateMessenger(app.allocator, app.instance, app.debugMessengerOptions())
	if err != nil {
		return err
	}

	return nil
}

func (app *HelloTriangleApplication) createSurface() error {
	surface, _, err := ext_surface_sdl2.CreateSurface(app.allocator, app.instance, &ext_surface_sdl2.CreationOptions{
		Window: app.window,
	})
	if err != nil {
		return err
	}

	app.surface = surface
	return nil
}

func (app *HelloTriangleApplication) pickPhysicalDevice() error {
	physicalDevices, _, err := app.instance.PhysicalDevices(app.allocator)
	if err != nil {
		return err
	}

	for _, device := range physicalDevices {
		if app.isDeviceSuitable(device) {
			app.physicalDevice = device
			break
		}
	}

	if app.physicalDevice == nil {
		return stacktrace.NewError("failed to find a suitable GPU!")
	}

	return nil
}

func (app *HelloTriangleApplication) createLogicalDevice() error {
	indices, err := app.findQueueFamilies(app.physicalDevice)
	if err != nil {
		return err
	}

	uniqueQueueFamilies := []int{*indices.GraphicsFamily}
	if uniqueQueueFamilies[0] != *indices.PresentFamily {
		uniqueQueueFamilies = append(uniqueQueueFamilies, *indices.PresentFamily)
	}

	var queueFamilyOptions []*core.QueueFamilyOptions
	queuePriority := float32(1.0)
	for _, queueFamily := range uniqueQueueFamilies {
		queueFamilyOptions = append(queueFamilyOptions, &core.QueueFamilyOptions{
			QueueFamilyIndex: queueFamily,
			QueuePriorities:  []float32{queuePriority},
		})
	}

	var extensionNames []string
	extensionNames = append(extensionNames, deviceExtensions...)

	var layerNames []string
	if enableValidationLayers {
		layerNames = append(layerNames, validationLayers...)
	}

	app.device, _, err = app.physicalDevice.CreateDevice(app.allocator, &core.DeviceOptions{
		QueueFamilies:   queueFamilyOptions,
		EnabledFeatures: &VKng.PhysicalDeviceFeatures{},
		ExtensionNames:  extensionNames,
		LayerNames:      layerNames,
	})
	if err != nil {
		return err
	}

	app.graphicsQueue, err = app.device.GetQueue(*indices.GraphicsFamily, 0)
	if err != nil {
		return err
	}

	app.presentQueue, err = app.device.GetQueue(*indices.PresentFamily, 0)
	return err
}

func (app *HelloTriangleApplication) createSwapchain() error {
	swapchainSupport, err := app.querySwapChainSupport(app.physicalDevice)
	if err != nil {
		return err
	}

	surfaceFormat := app.chooseSwapSurfaceFormat(swapchainSupport.Formats)
	presentMode := app.chooseSwapPresentMode(swapchainSupport.PresentModes)
	extent := app.chooseSwapExtent(swapchainSupport.Capabilities)

	imageCount := swapchainSupport.Capabilities.MinImageCount + 1
	if swapchainSupport.Capabilities.MaxImageCount > 0 && swapchainSupport.Capabilities.MaxImageCount < imageCount {
		imageCount = swapchainSupport.Capabilities.MaxImageCount
	}

	sharingMode := VKng.SharingExclusive
	var queueFamilyIndices []int

	indices, err := app.findQueueFamilies(app.physicalDevice)
	if err != nil {
		return err
	}

	if *indices.GraphicsFamily != *indices.PresentFamily {
		sharingMode = VKng.SharingConcurrent
		queueFamilyIndices = append(queueFamilyIndices, *indices.GraphicsFamily, *indices.PresentFamily)
	}

	swapchain, _, err := ext_swapchain.CreateSwapchain(app.allocator, app.device, &ext_swapchain.CreationOptions{
		Surface: app.surface,

		MinImageCount:    imageCount,
		ImageFormat:      surfaceFormat.Format,
		ImageColorSpace:  surfaceFormat.ColorSpace,
		ImageExtent:      extent,
		ImageArrayLayers: 1,
		ImageUsage:       VKng.ImageColorAttachment,

		SharingMode:        sharingMode,
		QueueFamilyIndices: queueFamilyIndices,

		PreTransform:   swapchainSupport.Capabilities.CurrentTransform,
		CompositeAlpha: ext_surface.Opaque,
		PresentMode:    presentMode,
		Clipped:        true,
	})
	if err != nil {
		return err
	}
	app.swapchainExtent = extent
	app.swapchain = swapchain
	app.swapchainImageFormat = surfaceFormat.Format

	images, _, err := swapchain.Images(app.allocator)
	if err != nil {
		return err
	}
	app.swapchainImages = images

	var imageViews []*core.ImageView
	for _, image := range images {
		view, _, err := app.device.CreateImageView(app.allocator, &core.ImageViewOptions{
			ViewType: VKng.View2D,
			Image:    image,
			Format:   surfaceFormat.Format,
			Components: VKng.ComponentMapping{
				R: VKng.SwizzleIdentity,
				G: VKng.SwizzleIdentity,
				B: VKng.SwizzleIdentity,
				A: VKng.SwizzleIdentity,
			},
			SubresourceRange: VKng.ImageSubresourceRange{
				AspectMask:     VKng.AspectColor,
				BaseMipLevel:   0,
				LevelCount:     1,
				BaseArrayLayer: 0,
				LayerCount:     1,
			},
		})
		if err != nil {
			return err
		}

		imageViews = append(imageViews, view)
	}
	app.swapchainImageViews = imageViews

	return nil
}

func (app *HelloTriangleApplication) createRenderPass() error {
	renderPass, _, err := render_pass.CreateRenderPass(app.allocator, app.device, &render_pass.RenderPassOptions{
		Attachments: []render_pass.AttachmentDescription{
			{
				Format:         app.swapchainImageFormat,
				Samples:        VKng.Samples1,
				LoadOp:         VKng.LoadOpClear,
				StoreOp:        VKng.StoreOpStore,
				StencilLoadOp:  VKng.LoadOpDontCare,
				StencilStoreOp: VKng.StoreOpDontCare,
				InitialLayout:  VKng.LayoutUndefined,
				FinalLayout:    VKng.LayoutPresentSrc,
			},
		},
		SubPasses: []render_pass.SubPass{
			{
				BindPoint: VKng.BindGraphics,
				ColorAttachments: []VKng.AttachmentReference{
					{
						AttachmentIndex: 0,
						Layout:          VKng.LayoutColorAttachmentOptimal,
					},
				},
			},
		},
		SubPassDependencies: []render_pass.SubPassDependency{
			{
				SrcSubPassIndex: render_pass.SubpassExternal,
				DstSubPassIndex: 0,

				SrcStageMask: VKng.PipelineStageColorAttachmentOutput,
				SrcAccess:    0,

				DstStageMask: VKng.PipelineStageColorAttachmentOutput,
				DstAccess:    VKng.AccessColorAttachmentWrite,
			},
		},
	})
	if err != nil {
		return err
	}

	app.renderPass = renderPass

	return nil
}

func bytesToBytecode(b []byte) []uint32 {
	byteCode := make([]uint32, len(b)/4)
	for i := 0; i < len(byteCode); i++ {
		byteIndex := i * 4
		byteCode[i] = 0
		byteCode[i] |= uint32(b[byteIndex])
		byteCode[i] |= uint32(b[byteIndex+1]) << 8
		byteCode[i] |= uint32(b[byteIndex+2]) << 16
		byteCode[i] |= uint32(b[byteIndex+3]) << 24
	}

	return byteCode
}

func (app *HelloTriangleApplication) createGraphicsPipeline() error {
	// Load vertex shader
	vertShaderBytes, err := shaders.ReadFile("shaders/vert.spv")
	if err != nil {
		return err
	}

	vertShader, _, err := app.device.CreateShaderModule(app.allocator, &core.ShaderModuleOptions{
		SpirVByteCode: bytesToBytecode(vertShaderBytes),
	})
	if err != nil {
		return err
	}
	defer vertShader.Destroy()

	// Load fragment shader
	fragShaderBytes, err := shaders.ReadFile("shaders/frag.spv")
	if err != nil {
		return err
	}

	fragShader, _, err := app.device.CreateShaderModule(app.allocator, &core.ShaderModuleOptions{
		SpirVByteCode: bytesToBytecode(fragShaderBytes),
	})
	if err != nil {
		return err
	}
	defer fragShader.Destroy()

	vertexInput := &pipeline.VertexInputOptions{}

	inputAssembly := &pipeline.InputAssemblyOptions{
		Topology:               VKng.TopologyTriangleList,
		EnablePrimitiveRestart: false,
	}

	vertStage := &pipeline.ShaderStage{
		Stage:  VKng.StageVertex,
		Shader: vertShader,
		Name:   "main",
	}

	fragStage := &pipeline.ShaderStage{
		Stage:  VKng.StageFragment,
		Shader: fragShader,
		Name:   "main",
	}

	viewport := &pipeline.ViewportOptions{
		Viewports: []VKng.Viewport{
			{
				X:        0,
				Y:        0,
				Width:    float32(app.swapchainExtent.Width),
				Height:   float32(app.swapchainExtent.Height),
				MinDepth: 0,
				MaxDepth: 1,
			},
		},
		Scissors: []VKng.Rect2D{
			{
				Offset: VKng.Offset2D{X: 0, Y: 0},
				Extent: app.swapchainExtent,
			},
		},
	}

	rasterization := &pipeline.RasterizationOptions{
		DepthClamp:        false,
		RasterizerDiscard: false,

		PolygonMode: pipeline.ModeFill,
		CullMode:    VKng.CullBack,
		FrontFace:   VKng.Clockwise,

		DepthBias: false,

		LineWidth: 1.0,
	}

	multisample := &pipeline.MultisampleOptions{
		SampleShading:        false,
		RasterizationSamples: VKng.Samples1,
		MinSampleShading:     1.0,
	}

	colorBlend := &pipeline.ColorBlendOptions{
		LogicOpEnabled: false,
		LogicOp:        VKng.LogicOpCopy,

		BlendConstants: [4]float32{0, 0, 0, 0},
		Attachments: []pipeline.ColorBlendAttachment{
			{
				BlendEnabled: false,
				WriteMask:    VKng.ComponentRed | VKng.ComponentGreen | VKng.ComponentBlue | VKng.ComponentAlpha,
			},
		},
	}

	app.pipelineLayout, _, err = pipeline.CreatePipelineLayout(app.allocator, app.device, &pipeline.PipelineLayoutOptions{})
	if err != nil {
		return err
	}

	pipelines, _, err := pipeline.CreateGraphicsPipelines(app.allocator, app.device, []*pipeline.Options{
		{
			ShaderStages: []*pipeline.ShaderStage{
				vertStage,
				fragStage,
			},
			VertexInput:       vertexInput,
			InputAssembly:     inputAssembly,
			Viewport:          viewport,
			Rasterization:     rasterization,
			Multisample:       multisample,
			ColorBlend:        colorBlend,
			Layout:            app.pipelineLayout,
			RenderPass:        app.renderPass,
			SubPass:           0,
			BasePipelineIndex: -1,
		},
	})
	if err != nil {
		return err
	}
	app.graphicsPipeline = pipelines[0]

	return nil
}

func (app *HelloTriangleApplication) createFramebuffers() error {
	for _, imageView := range app.swapchainImageViews {
		framebuffer, _, err := render_pass.CreateFrameBuffer(app.allocator, app.device, &render_pass.FramebufferOptions{
			RenderPass: app.renderPass,
			Layers:     1,
			Attachments: []*core.ImageView{
				imageView,
			},
			Width:  app.swapchainExtent.Width,
			Height: app.swapchainExtent.Height,
		})
		if err != nil {
			return err
		}

		app.swapchainFramebuffers = append(app.swapchainFramebuffers, framebuffer)
	}

	return nil
}

func (app *HelloTriangleApplication) createCommandPool() error {
	indices, err := app.findQueueFamilies(app.physicalDevice)
	if err != nil {
		return err
	}

	pool, _, err := commands.CreateCommandPool(app.allocator, app.device, &commands.CommandPoolOptions{
		GraphicsQueueFamily: indices.GraphicsFamily,
	})

	if err != nil {
		return err
	}
	app.commandPool = pool

	return nil
}

func (app *HelloTriangleApplication) createCommandBuffers() error {

	buffers, _, err := commands.CreateCommandBuffers(app.allocator, app.device, &commands.CommandBufferOptions{
		Level:       VKng.LevelPrimary,
		BufferCount: len(app.swapchainImages),
		CommandPool: app.commandPool,
	})
	if err != nil {
		return err
	}
	app.commandBuffers = buffers

	for bufferIdx, buffer := range buffers {
		_, err = buffer.Begin(app.allocator, &commands.BeginOptions{})
		if err != nil {
			return err
		}

		err = buffer.CmdBeginRenderPass(app.allocator, commands.ContentsInline,
			&commands.RenderPassBeginOptions{
				RenderPass:  app.renderPass,
				Framebuffer: app.swapchainFramebuffers[bufferIdx],
				RenderArea: VKng.Rect2D{
					Offset: VKng.Offset2D{X: 0, Y: 0},
					Extent: app.swapchainExtent,
				},
				ClearValues: []commands.ClearValue{
					commands.ClearValueFloat{0, 0, 0, 1},
				},
			})
		if err != nil {
			return err
		}

		buffer.CmdBindPipeline(VKng.BindGraphics, app.graphicsPipeline)
		buffer.CmdDraw(3, 1, 0, 0)
		buffer.CmdEndRenderPass()

		_, err = buffer.End()
		if err != nil {
			return err
		}
	}

	return nil
}

func (app *HelloTriangleApplication) createSyncObjects() error {
	for i := 0; i < MaxFramesInFlight; i++ {
		semaphore, _, err := app.device.CreateSemaphore(app.allocator, &core.SemaphoreOptions{})
		if err != nil {
			return err
		}

		app.imageAvailableSemaphore = append(app.imageAvailableSemaphore, semaphore)

		semaphore, _, err = app.device.CreateSemaphore(app.allocator, &core.SemaphoreOptions{})
		if err != nil {
			return err
		}

		app.renderFinishedSemaphore = append(app.renderFinishedSemaphore, semaphore)

		fence, _, err := app.device.CreateFence(app.allocator, &core.FenceOptions{
			Flags: core.FenceSignaled,
		})
		if err != nil {
			return err
		}

		app.inFlightFence = append(app.inFlightFence, fence)
	}

	for i := 0; i < len(app.swapchainImages); i++ {
		app.imagesInFlight = append(app.imagesInFlight, nil)
	}

	return nil
}

func (app *HelloTriangleApplication) drawFrame() error {
	fences := []*core.Fence{app.inFlightFence[app.currentFrame]}

	_, err := app.device.WaitForFences(app.allocator, true, VKng.NoTimeout, fences)
	if err != nil {
		return err
	}

	imageIndex, _, err := app.swapchain.AcquireNextImage(VKng.NoTimeout, app.imageAvailableSemaphore[app.currentFrame], nil)
	if err != nil {
		return err
	}

	if app.imagesInFlight[imageIndex] != nil {
		_, err := app.device.WaitForFences(app.allocator, true, VKng.NoTimeout, []*core.Fence{app.imagesInFlight[imageIndex]})
		if err != nil {
			return err
		}
	}
	app.imagesInFlight[imageIndex] = app.inFlightFence[app.currentFrame]

	_, err = app.device.ResetFences(app.allocator, fences)
	if err != nil {
		return err
	}

	_, err = commands.SubmitToQueue(app.allocator, app.graphicsQueue, app.inFlightFence[app.currentFrame], []*commands.SubmitOptions{
		{
			WaitSemaphores:   []*core.Semaphore{app.imageAvailableSemaphore[app.currentFrame]},
			WaitDstStages:    []VKng.PipelineStages{VKng.PipelineStageColorAttachmentOutput},
			CommandBuffers:   []*commands.CommandBuffer{app.commandBuffers[imageIndex]},
			SignalSemaphores: []*core.Semaphore{app.renderFinishedSemaphore[app.currentFrame]},
		},
	})
	if err != nil {
		return err
	}

	_, _, err = ext_swapchain.PresentToQueue(app.allocator, app.presentQueue, &ext_swapchain.PresentOptions{
		WaitSemaphores: []*core.Semaphore{app.renderFinishedSemaphore[app.currentFrame]},
		Swapchains:     []*ext_swapchain.Swapchain{app.swapchain},
		ImageIndices:   []int{imageIndex},
	})
	if err != nil {
		return err
	}

	app.currentFrame = (app.currentFrame + 1) % MaxFramesInFlight

	return nil
}

func (app *HelloTriangleApplication) chooseSwapSurfaceFormat(availableFormats []ext_surface.Format) ext_surface.Format {
	for _, format := range availableFormats {
		if format.Format == VKng.FormatB8G8R8A8SRGB && format.ColorSpace == ext_surface.SRGBNonlinear {
			return format
		}
	}

	return availableFormats[0]
}

func (app *HelloTriangleApplication) chooseSwapPresentMode(availablePresentModes []ext_surface.PresentMode) ext_surface.PresentMode {
	for _, presentMode := range availablePresentModes {
		if presentMode == ext_surface.Mailbox {
			return presentMode
		}
	}

	return ext_surface.FIFO
}

func (app *HelloTriangleApplication) chooseSwapExtent(capabilities *ext_surface.Capabilities) VKng.Extent2D {
	if capabilities.CurrentExtent.Width != (^uint32(0)) {
		return capabilities.CurrentExtent
	}

	widthInt, heightInt := app.window.VulkanGetDrawableSize()
	width := uint32(widthInt)
	height := uint32(heightInt)

	if width < capabilities.MinImageExtent.Width {
		width = capabilities.MinImageExtent.Width
	}
	if width > capabilities.MaxImageExtent.Width {
		width = capabilities.MaxImageExtent.Width
	}
	if height < capabilities.MinImageExtent.Height {
		height = capabilities.MinImageExtent.Height
	}
	if height > capabilities.MaxImageExtent.Height {
		height = capabilities.MaxImageExtent.Height
	}

	return VKng.Extent2D{Width: width, Height: height}
}

func (app *HelloTriangleApplication) querySwapChainSupport(device *core.PhysicalDevice) (SwapChainSupportDetails, error) {
	var details SwapChainSupportDetails
	var err error

	details.Capabilities, _, err = app.surface.Capabilities(app.allocator, device)
	if err != nil {
		return details, err
	}

	details.Formats, _, err = app.surface.Formats(app.allocator, device)
	if err != nil {
		return details, err
	}

	details.PresentModes, _, err = app.surface.PresentModes(app.allocator, device)
	return details, err
}

func (app *HelloTriangleApplication) isDeviceSuitable(device *core.PhysicalDevice) bool {
	indices, err := app.findQueueFamilies(device)
	if err != nil {
		return false
	}

	extensionsSupported := app.checkDeviceExtensionSupport(device)

	var swapChainAdequate bool
	if extensionsSupported {
		swapChainSupport, err := app.querySwapChainSupport(device)
		if err != nil {
			return false
		}

		swapChainAdequate = len(swapChainSupport.Formats) > 0 && len(swapChainSupport.PresentModes) > 0
	}

	return indices.IsComplete() && extensionsSupported && swapChainAdequate
}

func (app *HelloTriangleApplication) checkDeviceExtensionSupport(device *core.PhysicalDevice) bool {
	extensions, _, err := device.AvailableExtensions(app.allocator)
	if err != nil {
		return false
	}

	for _, extension := range deviceExtensions {
		_, hasExtension := extensions[extension]
		if !hasExtension {
			return false
		}
	}

	return true
}

func (app *HelloTriangleApplication) findQueueFamilies(device *core.PhysicalDevice) (QueueFamilyIndices, error) {
	indices := QueueFamilyIndices{}
	queueFamilies, err := device.QueueFamilyProperties(app.allocator)
	if err != nil {
		return indices, err
	}

	for queueFamilyIdx, queueFamily := range queueFamilies {
		if (queueFamily.Flags & VKng.Graphics) != 0 {
			indices.GraphicsFamily = new(int)
			*indices.GraphicsFamily = queueFamilyIdx
		}

		supported, _, err := app.surface.SupportsDevice(device, queueFamilyIdx)
		if err != nil {
			return indices, err
		}

		if supported {
			indices.PresentFamily = new(int)
			*indices.PresentFamily = queueFamilyIdx
		}

		if indices.IsComplete() {
			break
		}
	}

	return indices, nil
}

func (app *HelloTriangleApplication) logDebug(msgType ext_debugutils.MessageType, severity ext_debugutils.MessageSeverity, data *ext_debugutils.CallbackData) bool {
	log.Printf("[%s %s] - %s", severity, msgType, data.Message)
	return false
}

func main() {
	defAlloc := &cgoalloc.DefaultAllocator{}
	lowTier, err := cgoalloc.CreateFixedBlockAllocator(defAlloc, 64*1024, 64, 8)
	if err != nil {
		log.Fatalln(err)
	}

	highTier, err := cgoalloc.CreateFixedBlockAllocator(defAlloc, 4096*1024, 4096, 8)
	if err != nil {
		log.Fatalln(err)
	}

	alloc := cgoalloc.CreateFallbackAllocator(highTier, defAlloc)
	alloc = cgoalloc.CreateFallbackAllocator(lowTier, alloc)

	app := &HelloTriangleApplication{
		allocator: alloc,
	}

	err = app.Run()
	if err != nil {
		log.Fatalln(err)
	}
}

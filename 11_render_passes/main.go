package main

import (
	"embed"
	"github.com/CannibalVox/VKng/core"
	"github.com/CannibalVox/VKng/core/common"
	"github.com/CannibalVox/VKng/core/core1_0"
	"github.com/CannibalVox/VKng/extensions/ext_debug_utils"
	"github.com/CannibalVox/VKng/extensions/khr_surface"
	"github.com/CannibalVox/VKng/extensions/khr_surface_sdl2"
	"github.com/CannibalVox/VKng/extensions/khr_swapchain"
	"github.com/cockroachdb/errors"
	"github.com/veandco/go-sdl2/sdl"
	"log"
)

//go:embed shaders
var shaders embed.FS

var validationLayers = []string{"VK_LAYER_KHRONOS_validation"}
var deviceExtensions = []string{khr_swapchain.ExtensionName}

const enableValidationLayers = true

type QueueFamilyIndices struct {
	GraphicsFamily *int
	PresentFamily  *int
}

func (i *QueueFamilyIndices) IsComplete() bool {
	return i.GraphicsFamily != nil && i.PresentFamily != nil
}

type SwapChainSupportDetails struct {
	Capabilities *khr_surface.Capabilities
	Formats      []khr_surface.Format
	PresentModes []khr_surface.PresentMode
}

type HelloTriangleApplication struct {
	window *sdl.Window
	loader core.Loader

	instance       core1_0.Instance
	debugMessenger ext_debug_utils.Messenger
	surface        khr_surface.Surface

	physicalDevice core1_0.PhysicalDevice
	device         core1_0.Device

	graphicsQueue core1_0.Queue
	presentQueue  core1_0.Queue

	swapchainExtension   khr_swapchain.Extension
	swapchain            khr_swapchain.Swapchain
	swapchainImages      []core1_0.Image
	swapchainImageFormat common.DataFormat
	swapchainExtent      common.Extent2D
	swapchainImageViews  []core1_0.ImageView

	renderPass     core1_0.RenderPass
	pipelineLayout core1_0.PipelineLayout
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

	app.loader, err = core.CreateLoaderFromProcAddr(sdl.VulkanGetVkGetInstanceProcAddr())
	if err != nil {
		return err
	}

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

	return app.createGraphicsPipeline()
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
	}

	return nil
}

func (app *HelloTriangleApplication) cleanup() {
	if app.pipelineLayout != nil {
		app.pipelineLayout.Destroy(nil)
	}

	if app.renderPass != nil {
		app.renderPass.Destroy(nil)
	}

	for _, imageView := range app.swapchainImageViews {
		imageView.Destroy(nil)
	}

	if app.swapchain != nil {
		app.swapchain.Destroy(nil)
	}

	if app.device != nil {
		app.device.Destroy(nil)
	}

	if app.debugMessenger != nil {
		app.debugMessenger.Destroy(nil)
	}

	if app.surface != nil {
		app.surface.Destroy(nil)
	}

	if app.instance != nil {
		app.instance.Destroy(nil)
	}

	if app.window != nil {
		app.window.Destroy()
	}
	sdl.Quit()
}

func (app *HelloTriangleApplication) createInstance() error {
	instanceOptions := core1_0.InstanceOptions{
		ApplicationName:    "Hello Triangle",
		ApplicationVersion: common.CreateVersion(1, 0, 0),
		EngineName:         "No Engine",
		EngineVersion:      common.CreateVersion(1, 0, 0),
		VulkanVersion:      common.Vulkan1_2,
	}

	// Add extensions
	sdlExtensions := app.window.VulkanGetInstanceExtensions()
	extensions, _, err := app.loader.AvailableExtensions()
	if err != nil {
		return err
	}

	for _, ext := range sdlExtensions {
		_, hasExt := extensions[ext]
		if !hasExt {
			return errors.Newf("createinstance: cannot initialize sdl: missing extension %s", ext)
		}
		instanceOptions.ExtensionNames = append(instanceOptions.ExtensionNames, ext)
	}

	if enableValidationLayers {
		instanceOptions.ExtensionNames = append(instanceOptions.ExtensionNames, ext_debug_utils.ExtensionName)
	}

	// Add layers
	layers, _, err := app.loader.AvailableLayers()
	if err != nil {
		return err
	}

	if enableValidationLayers {
		for _, layer := range validationLayers {
			_, hasValidation := layers[layer]
			if !hasValidation {
				return errors.Newf("createInstance: cannot add validation- layer %s not available- install LunarG Vulkan SDK", layer)
			}
			instanceOptions.LayerNames = append(instanceOptions.LayerNames, layer)
		}

		// Add debug messenger
		instanceOptions.Next = app.debugMessengerOptions()
	}

	app.instance, _, err = app.loader.CreateInstance(nil, instanceOptions)
	if err != nil {
		return err
	}

	return nil
}

func (app *HelloTriangleApplication) debugMessengerOptions() ext_debug_utils.CreateOptions {
	return ext_debug_utils.CreateOptions{
		CaptureSeverities: ext_debug_utils.SeverityError | ext_debug_utils.SeverityWarning,
		CaptureTypes:      ext_debug_utils.TypeGeneral | ext_debug_utils.TypeValidation | ext_debug_utils.TypePerformance,
		Callback:          app.logDebug,
	}
}

func (app *HelloTriangleApplication) setupDebugMessenger() error {
	if !enableValidationLayers {
		return nil
	}

	var err error
	debugLoader := ext_debug_utils.CreateExtensionFromInstance(app.instance)
	app.debugMessenger, _, err = debugLoader.CreateMessenger(app.instance, nil, app.debugMessengerOptions())
	if err != nil {
		return err
	}

	return nil
}

func (app *HelloTriangleApplication) createSurface() error {
	surfaceLoader := khr_surface_sdl2.CreateExtensionFromInstance(app.instance)
	surface, _, err := surfaceLoader.CreateSurface(app.instance, app.window)
	if err != nil {
		return err
	}

	app.surface = surface
	return nil
}

func (app *HelloTriangleApplication) pickPhysicalDevice() error {
	physicalDevices, _, err := app.loader.PhysicalDevices(app.instance)
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
		return errors.New("failed to find a suitable GPU!")
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

	var queueFamilyOptions []core1_0.QueueFamilyOptions
	queuePriority := float32(1.0)
	for _, queueFamily := range uniqueQueueFamilies {
		queueFamilyOptions = append(queueFamilyOptions, core1_0.QueueFamilyOptions{
			QueueFamilyIndex: queueFamily,
			QueuePriorities:  []float32{queuePriority},
		})
	}

	var extensionNames []string
	extensionNames = append(extensionNames, deviceExtensions...)

	// Makes this example compatible with vulkan portability, necessary to run on mobile & mac
	extensions, _, err := app.physicalDevice.AvailableExtensions()
	if err != nil {
		return err
	}

	_, supported := extensions["VK_KHR_portability_subset"]
	if supported {
		extensionNames = append(extensionNames, "VK_KHR_portability_subset")
	}

	var layerNames []string
	if enableValidationLayers {
		layerNames = append(layerNames, validationLayers...)
	}

	app.device, _, err = app.loader.CreateDevice(app.physicalDevice, nil, core1_0.DeviceOptions{
		QueueFamilies:   queueFamilyOptions,
		EnabledFeatures: &core1_0.PhysicalDeviceFeatures{},
		ExtensionNames:  extensionNames,
		LayerNames:      layerNames,
	})
	if err != nil {
		return err
	}

	app.graphicsQueue = app.loader.GetQueue(app.device, *indices.GraphicsFamily, 0)
	app.presentQueue = app.loader.GetQueue(app.device, *indices.PresentFamily, 0)
	return nil
}

func (app *HelloTriangleApplication) createSwapchain() error {
	app.swapchainExtension = khr_swapchain.CreateExtensionFromDevice(app.device)

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

	sharingMode := core1_0.SharingExclusive
	var queueFamilyIndices []int

	indices, err := app.findQueueFamilies(app.physicalDevice)
	if err != nil {
		return err
	}

	if *indices.GraphicsFamily != *indices.PresentFamily {
		sharingMode = core1_0.SharingConcurrent
		queueFamilyIndices = append(queueFamilyIndices, *indices.GraphicsFamily, *indices.PresentFamily)
	}

	swapchain, _, err := app.swapchainExtension.CreateSwapchain(app.device, nil, khr_swapchain.CreateOptions{
		Surface: app.surface,

		MinImageCount:    imageCount,
		ImageFormat:      surfaceFormat.Format,
		ImageColorSpace:  surfaceFormat.ColorSpace,
		ImageExtent:      extent,
		ImageArrayLayers: 1,
		ImageUsage:       core1_0.ImageUsageColorAttachment,

		SharingMode:        sharingMode,
		QueueFamilyIndices: queueFamilyIndices,

		PreTransform:   swapchainSupport.Capabilities.CurrentTransform,
		CompositeAlpha: khr_surface.CompositeAlphaModeOpaque,
		PresentMode:    presentMode,
		Clipped:        true,
	})
	if err != nil {
		return err
	}
	app.swapchainExtent = extent
	app.swapchain = swapchain

	images, _, err := swapchain.Images()
	if err != nil {
		return err
	}
	app.swapchainImages = images

	var imageViews []core1_0.ImageView
	for _, image := range images {
		view, _, err := app.loader.CreateImageView(app.device, nil, core1_0.ImageViewOptions{
			ViewType: core1_0.ViewType2D,
			Image:    image,
			Format:   surfaceFormat.Format,
			Components: core1_0.ComponentMapping{
				R: core1_0.SwizzleIdentity,
				G: core1_0.SwizzleIdentity,
				B: core1_0.SwizzleIdentity,
				A: core1_0.SwizzleIdentity,
			},
			SubresourceRange: common.ImageSubresourceRange{
				AspectMask:     core1_0.AspectColor,
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
	app.swapchainImageFormat = surfaceFormat.Format

	return nil
}

func (app *HelloTriangleApplication) createRenderPass() error {
	renderPass, _, err := app.loader.CreateRenderPass(app.device, nil, core1_0.RenderPassOptions{
		Attachments: []core1_0.AttachmentDescription{
			{
				Format:         app.swapchainImageFormat,
				Samples:        core1_0.Samples1,
				LoadOp:         core1_0.LoadOpClear,
				StoreOp:        core1_0.StoreOpStore,
				StencilLoadOp:  core1_0.LoadOpDontCare,
				StencilStoreOp: core1_0.StoreOpDontCare,
				InitialLayout:  core1_0.ImageLayoutUndefined,
				FinalLayout:    khr_swapchain.ImageLayoutPresentSrc,
			},
		},
		SubPasses: []core1_0.SubPass{
			{
				BindPoint: core1_0.BindGraphics,
				ColorAttachments: []common.AttachmentReference{
					{
						AttachmentIndex: 0,
						Layout:          core1_0.ImageLayoutColorAttachmentOptimal,
					},
				},
			},
		},
		SubPassDependencies: []core1_0.SubPassDependency{
			{
				SrcSubPassIndex: core1_0.SubpassExternal,
				DstSubPassIndex: 0,

				SrcStageMask:  core1_0.PipelineStageColorAttachmentOutput,
				SrcAccessMask: 0,

				DstStageMask:  core1_0.PipelineStageColorAttachmentOutput,
				DstAccessMask: core1_0.AccessColorAttachmentWrite,
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

	vertShader, _, err := app.loader.CreateShaderModule(app.device, nil, core1_0.ShaderModuleOptions{
		SpirVByteCode: bytesToBytecode(vertShaderBytes),
	})
	if err != nil {
		return err
	}
	defer vertShader.Destroy(nil)

	// Load fragment shader
	fragShaderBytes, err := shaders.ReadFile("shaders/frag.spv")
	if err != nil {
		return err
	}

	fragShader, _, err := app.loader.CreateShaderModule(app.device, nil, core1_0.ShaderModuleOptions{
		SpirVByteCode: bytesToBytecode(fragShaderBytes),
	})
	if err != nil {
		return err
	}
	defer fragShader.Destroy(nil)

	_ = &core1_0.VertexInputOptions{}

	_ = &core1_0.InputAssemblyOptions{
		Topology:               core1_0.TopologyTriangleList,
		EnablePrimitiveRestart: false,
	}

	_ = &core1_0.ShaderStageOptions{
		Stage:  core1_0.StageVertex,
		Shader: vertShader,
		Name:   "main",
	}

	_ = &core1_0.ShaderStageOptions{
		Stage:  core1_0.StageFragment,
		Shader: fragShader,
		Name:   "main",
	}

	_ = &core1_0.ViewportOptions{
		Viewports: []common.Viewport{
			{
				X:        0,
				Y:        0,
				Width:    float32(app.swapchainExtent.Width),
				Height:   float32(app.swapchainExtent.Height),
				MinDepth: 0,
				MaxDepth: 1,
			},
		},
		Scissors: []common.Rect2D{
			{
				Offset: common.Offset2D{X: 0, Y: 0},
				Extent: app.swapchainExtent,
			},
		},
	}

	_ = &core1_0.RasterizationOptions{
		DepthClamp:        false,
		RasterizerDiscard: false,

		PolygonMode: core1_0.PolygonModeFill,
		CullMode:    core1_0.CullBack,
		FrontFace:   core1_0.FrontFaceClockwise,

		DepthBias: false,

		LineWidth: 1.0,
	}

	_ = &core1_0.MultisampleOptions{
		SampleShading:        false,
		RasterizationSamples: core1_0.Samples1,
		MinSampleShading:     1.0,
	}

	_ = &core1_0.ColorBlendOptions{
		LogicOpEnabled: false,
		LogicOp:        core1_0.LogicOpCopy,

		BlendConstants: [4]float32{0, 0, 0, 0},
		Attachments: []core1_0.ColorBlendAttachment{
			{
				BlendEnabled: false,
				WriteMask:    common.ComponentRed | common.ComponentGreen | common.ComponentBlue | common.ComponentAlpha,
			},
		},
	}

	app.pipelineLayout, _, err = app.loader.CreatePipelineLayout(app.device, nil, core1_0.PipelineLayoutOptions{})
	if err != nil {
		return err
	}

	return nil
}

func (app *HelloTriangleApplication) chooseSwapSurfaceFormat(availableFormats []khr_surface.Format) khr_surface.Format {
	for _, format := range availableFormats {
		if format.Format == core1_0.DataFormatB8G8R8A8SRGB && format.ColorSpace == khr_surface.ColorSpaceSRGBNonlinear {
			return format
		}
	}

	return availableFormats[0]
}

func (app *HelloTriangleApplication) chooseSwapPresentMode(availablePresentModes []khr_surface.PresentMode) khr_surface.PresentMode {
	for _, presentMode := range availablePresentModes {
		if presentMode == khr_surface.PresentMailbox {
			return presentMode
		}
	}

	return khr_surface.PresentFIFO
}

func (app *HelloTriangleApplication) chooseSwapExtent(capabilities *khr_surface.Capabilities) common.Extent2D {
	if capabilities.CurrentExtent.Width != -1 {
		return capabilities.CurrentExtent
	}

	widthInt, heightInt := app.window.VulkanGetDrawableSize()
	width := int(widthInt)
	height := int(heightInt)

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

	return common.Extent2D{Width: width, Height: height}
}

func (app *HelloTriangleApplication) querySwapChainSupport(device core1_0.PhysicalDevice) (SwapChainSupportDetails, error) {
	var details SwapChainSupportDetails
	var err error

	details.Capabilities, _, err = app.surface.Capabilities(device)
	if err != nil {
		return details, err
	}

	details.Formats, _, err = app.surface.Formats(device)
	if err != nil {
		return details, err
	}

	details.PresentModes, _, err = app.surface.PresentModes(device)
	return details, err
}

func (app *HelloTriangleApplication) isDeviceSuitable(device core1_0.PhysicalDevice) bool {
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

func (app *HelloTriangleApplication) checkDeviceExtensionSupport(device core1_0.PhysicalDevice) bool {
	extensions, _, err := device.AvailableExtensions()
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

func (app *HelloTriangleApplication) findQueueFamilies(device core1_0.PhysicalDevice) (QueueFamilyIndices, error) {
	indices := QueueFamilyIndices{}
	queueFamilies := device.QueueFamilyProperties()

	for queueFamilyIdx, queueFamily := range queueFamilies {
		if (queueFamily.Flags & core1_0.QueueGraphics) != 0 {
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

func (app *HelloTriangleApplication) logDebug(msgType ext_debug_utils.MessageTypes, severity ext_debug_utils.MessageSeverities, data *ext_debug_utils.CallbackDataOptions) bool {
	log.Printf("[%s %s] - %s", severity, msgType, data.Message)
	return false
}

func main() {
	app := &HelloTriangleApplication{}

	err := app.Run()
	if err != nil {
		log.Fatalf("%+v\n", err)
	}
}

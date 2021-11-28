package main

import (
	"bytes"
	"embed"
	"encoding/binary"
	"github.com/CannibalVox/VKng/core"
	"github.com/CannibalVox/VKng/core/common"
	"github.com/CannibalVox/VKng/extensions/ext_debug_utils"
	"github.com/CannibalVox/VKng/extensions/khr_surface"
	"github.com/CannibalVox/VKng/extensions/khr_surface_sdl2"
	"github.com/CannibalVox/VKng/extensions/khr_swapchain"
	"github.com/cockroachdb/errors"
	"github.com/g3n/engine/loader/obj"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/loov/hrtime"
	"github.com/palantir/stacktrace"
	"github.com/veandco/go-sdl2/sdl"
	"image/png"
	"log"
	"math"
	"unsafe"
)

//go:embed shaders images meshes
var fileSystem embed.FS

const MaxFramesInFlight = 2

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

type Vertex struct {
	Position mgl32.Vec3
	Color    mgl32.Vec3
	TexCoord mgl32.Vec2
}

type UniformBufferObject struct {
	Model mgl32.Mat4
	View  mgl32.Mat4
	Proj  mgl32.Mat4
}

func getVertexBindingDescription() []core.VertexBindingDescription {
	v := Vertex{}
	return []core.VertexBindingDescription{
		{
			Binding:   0,
			Stride:    int(unsafe.Sizeof(v)),
			InputRate: core.RateVertex,
		},
	}
}

func getVertexAttributeDescriptions() []core.VertexAttributeDescription {
	v := Vertex{}
	return []core.VertexAttributeDescription{
		{
			Binding:  0,
			Location: 0,
			Format:   common.FormatR32G32B32SignedFloat,
			Offset:   int(unsafe.Offsetof(v.Position)),
		},
		{
			Binding:  0,
			Location: 1,
			Format:   common.FormatR32G32B32SignedFloat,
			Offset:   int(unsafe.Offsetof(v.Color)),
		},
		{
			Binding:  0,
			Location: 2,
			Format:   common.FormatR32G32SignedFloat,
			Offset:   int(unsafe.Offsetof(v.TexCoord)),
		},
	}
}

type HelloTriangleApplication struct {
	window *sdl.Window
	loader core.Loader1_0

	instance       core.Instance
	debugMessenger ext_debug_utils.Messenger
	surface        khr_surface.Surface

	physicalDevice core.PhysicalDevice
	device         core.Device

	graphicsQueue core.Queue
	presentQueue  core.Queue

	swapchainLoader       khr_swapchain.Loader
	swapchain             khr_swapchain.Swapchain
	swapchainImages       []core.Image
	swapchainImageFormat  common.DataFormat
	swapchainExtent       common.Extent2D
	swapchainImageViews   []core.ImageView
	swapchainFramebuffers []core.Framebuffer

	renderPass          core.RenderPass
	descriptorPool      core.DescriptorPool
	descriptorSets      []core.DescriptorSet
	descriptorSetLayout core.DescriptorSetLayout
	pipelineLayout      core.PipelineLayout
	graphicsPipeline    core.Pipeline

	commandPool    core.CommandPool
	commandBuffers []core.CommandBuffer

	imageAvailableSemaphore []core.Semaphore
	renderFinishedSemaphore []core.Semaphore
	inFlightFence           []core.Fence
	imagesInFlight          []core.Fence
	currentFrame            int
	frameStart              float64

	vertices           []Vertex
	indices            []uint32
	vertexBuffer       core.Buffer
	vertexBufferMemory core.DeviceMemory
	indexBuffer        core.Buffer
	indexBufferMemory  core.DeviceMemory

	uniformBuffers       []core.Buffer
	uniformBuffersMemory []core.DeviceMemory

	textureImage       core.Image
	textureImageMemory core.DeviceMemory
	textureImageView   core.ImageView
	textureSampler     core.Sampler

	depthImage       core.Image
	depthImageMemory core.DeviceMemory
	depthImageView   core.ImageView
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

	window, err := sdl.CreateWindow("Vulkan", sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED, 800, 600, sdl.WINDOW_SHOWN|sdl.WINDOW_VULKAN|sdl.WINDOW_RESIZABLE)
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

	err = app.createImageViews()
	if err != nil {
		return err
	}

	err = app.createRenderPass()
	if err != nil {
		return err
	}

	err = app.createDescriptorSetLayout()
	if err != nil {
		return err
	}

	err = app.createGraphicsPipeline()
	if err != nil {
		return err
	}

	err = app.createCommandPool()
	if err != nil {
		return err
	}

	err = app.createDepthResources()
	if err != nil {
		return err
	}

	err = app.createFramebuffers()
	if err != nil {
		return err
	}

	err = app.createTextureImage()
	if err != nil {
		return err
	}

	err = app.createTextureImageView()
	if err != nil {
		return err
	}

	err = app.createSampler()
	if err != nil {
		return err
	}

	err = app.loadModel()
	if err != nil {
		return err
	}
	err = app.createVertexBuffer()
	if err != nil {
		return err
	}

	err = app.createIndexBuffer()
	if err != nil {
		return err
	}

	err = app.createUniformBuffers()
	if err != nil {
		return err
	}

	err = app.createDescriptorPool()
	if err != nil {
		return err
	}

	err = app.createDescriptorSets()
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
	rendering := true

appLoop:
	for true {
		for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
			switch e := event.(type) {
			case *sdl.QuitEvent:
				break appLoop
			case *sdl.WindowEvent:
				switch e.Event {
				case sdl.WINDOWEVENT_MINIMIZED:
					rendering = false
				case sdl.WINDOWEVENT_RESTORED:
					rendering = true
				case sdl.WINDOWEVENT_RESIZED:
					w, h := app.window.GetSize()
					if w > 0 && h > 0 {
						rendering = true
						app.recreateSwapChain()
					} else {
						rendering = false
					}
				}
			}
		}
		if rendering {
			err := app.drawFrame()
			if err != nil {
				return err
			}
		}
	}

	_, err := app.device.WaitForIdle()
	return err
}

func (app *HelloTriangleApplication) cleanupSwapChain() {
	if app.depthImageView != nil {
		app.depthImageView.Destroy()
		app.depthImageView = nil
	}

	if app.depthImage != nil {
		app.depthImage.Destroy()
		app.depthImage = nil
	}

	if app.depthImageMemory != nil {
		app.device.FreeMemory(app.depthImageMemory)
		app.depthImageMemory = nil
	}

	for _, framebuffer := range app.swapchainFramebuffers {
		framebuffer.Destroy()
	}
	app.swapchainFramebuffers = []core.Framebuffer{}

	if len(app.commandBuffers) > 0 {
		app.commandPool.FreeCommandBuffers(app.commandBuffers)
		app.commandBuffers = []core.CommandBuffer{}
	}

	if app.graphicsPipeline != nil {
		app.graphicsPipeline.Destroy()
		app.graphicsPipeline = nil
	}

	if app.pipelineLayout != nil {
		app.pipelineLayout.Destroy()
		app.pipelineLayout = nil
	}

	if app.renderPass != nil {
		app.renderPass.Destroy()
		app.renderPass = nil
	}

	for _, imageView := range app.swapchainImageViews {
		imageView.Destroy()
	}
	app.swapchainImageViews = []core.ImageView{}

	if app.swapchain != nil {
		app.swapchain.Destroy()
		app.swapchain = nil
	}

	for i := 0; i < len(app.uniformBuffers); i++ {
		app.uniformBuffers[i].Destroy()
	}
	app.uniformBuffers = app.uniformBuffers[:0]

	for i := 0; i < len(app.uniformBuffersMemory); i++ {
		app.device.FreeMemory(app.uniformBuffersMemory[i])
	}
	app.uniformBuffersMemory = app.uniformBuffersMemory[:0]

	app.descriptorPool.Destroy()
}

func (app *HelloTriangleApplication) cleanup() {
	app.cleanupSwapChain()

	if app.textureSampler != nil {
		app.textureSampler.Destroy()
	}

	if app.textureImageView != nil {
		app.textureImageView.Destroy()
	}

	if app.textureImage != nil {
		app.textureImage.Destroy()
	}

	if app.textureImageMemory != nil {
		app.device.FreeMemory(app.textureImageMemory)
	}

	if app.descriptorSetLayout != nil {
		app.descriptorSetLayout.Destroy()
	}

	if app.indexBuffer != nil {
		app.indexBuffer.Destroy()
	}

	if app.indexBufferMemory != nil {
		app.device.FreeMemory(app.indexBufferMemory)
	}

	if app.vertexBuffer != nil {
		app.vertexBuffer.Destroy()
	}

	if app.vertexBufferMemory != nil {
		app.device.FreeMemory(app.vertexBufferMemory)
	}

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
}

func (app *HelloTriangleApplication) recreateSwapChain() error {
	w, h := app.window.VulkanGetDrawableSize()
	if w == 0 || h == 0 {
		return nil
	}
	if (app.window.GetFlags() & sdl.WINDOW_MINIMIZED) != 0 {
		return nil
	}

	_, err := app.device.WaitForIdle()
	if err != nil {
		return err
	}

	app.cleanupSwapChain()

	err = app.createSwapchain()
	if err != nil {
		return err
	}

	err = app.createImageViews()
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

	err = app.createDepthResources()
	if err != nil {
		return err
	}

	err = app.createFramebuffers()
	if err != nil {
		return err
	}

	err = app.createUniformBuffers()
	if err != nil {
		return err
	}

	err = app.createDescriptorPool()
	if err != nil {
		return err
	}

	err = app.createDescriptorSets()
	if err != nil {
		return err
	}

	err = app.createCommandBuffers()
	if err != nil {
		return err
	}

	app.imagesInFlight = []core.Fence{}
	for i := 0; i < len(app.swapchainImages); i++ {
		app.imagesInFlight = append(app.imagesInFlight, nil)
	}

	return nil
}

func (app *HelloTriangleApplication) createInstance() error {
	instanceOptions := &core.InstanceOptions{
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
			return stacktrace.NewError("createinstance: cannot initialize sdl: missing extension %s", ext)
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
				return stacktrace.NewError("createInstance: cannot add validation- layer %s not available- install LunarG Vulkan SDK", layer)
			}
			instanceOptions.LayerNames = append(instanceOptions.LayerNames, layer)
		}

		// Add debug messenger
		instanceOptions.Next = app.debugMessengerOptions()
	}

	app.instance, _, err = app.loader.CreateInstance(instanceOptions)
	if err != nil {
		return err
	}

	return nil
}

func (app *HelloTriangleApplication) debugMessengerOptions() *ext_debug_utils.CreationOptions {
	return &ext_debug_utils.CreationOptions{
		CaptureSeverities: ext_debug_utils.SeverityError | ext_debug_utils.SeverityWarning,
		CaptureTypes:      ext_debug_utils.TypeAll,
		Callback:          app.logDebug,
	}
}

func (app *HelloTriangleApplication) setupDebugMessenger() error {
	if !enableValidationLayers {
		return nil
	}

	var err error
	debugLoader := ext_debug_utils.CreateLoaderFromInstance(app.instance)
	app.debugMessenger, _, err = debugLoader.CreateMessenger(app.instance, app.debugMessengerOptions())
	if err != nil {
		return err
	}

	return nil
}

func (app *HelloTriangleApplication) createSurface() error {
	surfaceLoader := khr_surface_sdl2.CreateLoaderFromInstance(app.instance)
	surface, _, err := surfaceLoader.CreateSurface(app.instance, app.window)
	if err != nil {
		return err
	}

	app.surface = surface
	return nil
}

func (app *HelloTriangleApplication) pickPhysicalDevice() error {
	physicalDevices, _, err := app.instance.PhysicalDevices()
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

	app.device, _, err = app.loader.CreateDevice(app.physicalDevice, &core.DeviceOptions{
		QueueFamilies: queueFamilyOptions,
		EnabledFeatures: &common.PhysicalDeviceFeatures{
			SamplerAnisotropy: true,
		},
		ExtensionNames: extensionNames,
		LayerNames:     layerNames,
	})
	if err != nil {
		return err
	}

	app.graphicsQueue = app.device.GetQueue(*indices.GraphicsFamily, 0)
	app.presentQueue = app.device.GetQueue(*indices.PresentFamily, 0)
	return nil
}

func (app *HelloTriangleApplication) createSwapchain() error {
	app.swapchainLoader = khr_swapchain.CreateLoaderFromDevice(app.device)

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

	sharingMode := common.SharingExclusive
	var queueFamilyIndices []int

	indices, err := app.findQueueFamilies(app.physicalDevice)
	if err != nil {
		return err
	}

	if *indices.GraphicsFamily != *indices.PresentFamily {
		sharingMode = common.SharingConcurrent
		queueFamilyIndices = append(queueFamilyIndices, *indices.GraphicsFamily, *indices.PresentFamily)
	}

	swapchain, _, err := app.swapchainLoader.CreateSwapchain(app.device, &khr_swapchain.CreationOptions{
		Surface: app.surface,

		MinImageCount:    imageCount,
		ImageFormat:      surfaceFormat.Format,
		ImageColorSpace:  surfaceFormat.ColorSpace,
		ImageExtent:      extent,
		ImageArrayLayers: 1,
		ImageUsage:       common.ImageColorAttachment,

		SharingMode:        sharingMode,
		QueueFamilyIndices: queueFamilyIndices,

		PreTransform:   swapchainSupport.Capabilities.CurrentTransform,
		CompositeAlpha: khr_surface.AlphaModeOpaque,
		PresentMode:    presentMode,
		Clipped:        true,
	})
	if err != nil {
		return err
	}
	app.swapchainExtent = extent
	app.swapchain = swapchain
	app.swapchainImageFormat = surfaceFormat.Format

	return nil
}

func (app *HelloTriangleApplication) createImageViews() error {
	images, _, err := app.swapchain.Images()
	if err != nil {
		return err
	}
	app.swapchainImages = images

	var imageViews []core.ImageView
	for _, image := range images {
		view, err := app.createImageView(image, app.swapchainImageFormat, common.AspectColor)
		if err != nil {
			return err
		}

		imageViews = append(imageViews, view)
	}
	app.swapchainImageViews = imageViews

	return nil
}

func (app *HelloTriangleApplication) createRenderPass() error {
	depthFormat, err := app.findDepthFormat()
	if err != nil {
		return err
	}

	renderPass, _, err := app.loader.CreateRenderPass(app.device, &core.RenderPassOptions{
		Attachments: []core.AttachmentDescription{
			{
				Format:         app.swapchainImageFormat,
				Samples:        common.Samples1,
				LoadOp:         common.LoadOpClear,
				StoreOp:        common.StoreOpStore,
				StencilLoadOp:  common.LoadOpDontCare,
				StencilStoreOp: common.StoreOpDontCare,
				InitialLayout:  common.LayoutUndefined,
				FinalLayout:    common.LayoutPresentSrcKHR,
			},
			{
				Format:         depthFormat,
				Samples:        common.Samples1,
				LoadOp:         common.LoadOpClear,
				StoreOp:        common.StoreOpDontCare,
				StencilLoadOp:  common.LoadOpDontCare,
				StencilStoreOp: common.StoreOpDontCare,
				InitialLayout:  common.LayoutUndefined,
				FinalLayout:    common.LayoutDepthStencilAttachmentOptimal,
			},
		},
		SubPasses: []core.SubPass{
			{
				BindPoint: common.BindGraphics,
				ColorAttachments: []common.AttachmentReference{
					{
						AttachmentIndex: 0,
						Layout:          common.LayoutColorAttachmentOptimal,
					},
				},
				DepthStencilAttachment: &common.AttachmentReference{
					AttachmentIndex: 1,
					Layout:          common.LayoutDepthStencilAttachmentOptimal,
				},
			},
		},
		SubPassDependencies: []core.SubPassDependency{
			{
				SrcSubPassIndex: core.SubpassExternal,
				DstSubPassIndex: 0,

				SrcStageMask: common.PipelineStageColorAttachmentOutput | common.PipelineStageEarlyFragmentTests,
				SrcAccess:    0,

				DstStageMask: common.PipelineStageColorAttachmentOutput | common.PipelineStageEarlyFragmentTests,
				DstAccess:    common.AccessColorAttachmentWrite | common.AccessDepthStencilAttachmentWrite,
			},
		},
	})
	if err != nil {
		return err
	}

	app.renderPass = renderPass

	return nil
}

func (app *HelloTriangleApplication) createDescriptorSetLayout() error {
	var err error
	app.descriptorSetLayout, _, err = app.loader.CreateDescriptorSetLayout(app.device, &core.DescriptorSetLayoutOptions{
		Bindings: []*core.DescriptorLayoutBinding{
			{
				Binding: 0,
				Type:    common.DescriptorUniformBuffer,
				Count:   1,

				ShaderStages: common.StageVertex,
			},
			{
				Binding: 1,
				Type:    common.DescriptorCombinedImageSampler,
				Count:   1,

				ShaderStages: common.StageFragment,
			},
		},
	})
	if err != nil {
		return err
	}

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
	vertShaderBytes, err := fileSystem.ReadFile("shaders/vert.spv")
	if err != nil {
		return err
	}

	vertShader, _, err := app.loader.CreateShaderModule(app.device, &core.ShaderModuleOptions{
		SpirVByteCode: bytesToBytecode(vertShaderBytes),
	})
	if err != nil {
		return err
	}
	defer vertShader.Destroy()

	// Load fragment shader
	fragShaderBytes, err := fileSystem.ReadFile("shaders/frag.spv")
	if err != nil {
		return err
	}

	fragShader, _, err := app.loader.CreateShaderModule(app.device, &core.ShaderModuleOptions{
		SpirVByteCode: bytesToBytecode(fragShaderBytes),
	})
	if err != nil {
		return err
	}
	defer fragShader.Destroy()

	vertexInput := &core.VertexInputOptions{
		VertexBindingDescriptions:   getVertexBindingDescription(),
		VertexAttributeDescriptions: getVertexAttributeDescriptions(),
	}

	inputAssembly := &core.InputAssemblyOptions{
		Topology:               common.TopologyTriangleList,
		EnablePrimitiveRestart: false,
	}

	vertStage := &core.ShaderStage{
		Stage:  common.StageVertex,
		Shader: vertShader,
		Name:   "main",
	}

	fragStage := &core.ShaderStage{
		Stage:  common.StageFragment,
		Shader: fragShader,
		Name:   "main",
	}

	viewport := &core.ViewportOptions{
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

	rasterization := &core.RasterizationOptions{
		DepthClamp:        false,
		RasterizerDiscard: false,

		PolygonMode: core.ModeFill,
		CullMode:    common.CullBack,
		FrontFace:   common.FrontFaceCounterClockwise,

		DepthBias: false,

		LineWidth: 1.0,
	}

	multisample := &core.MultisampleOptions{
		SampleShading:        false,
		RasterizationSamples: common.Samples1,
		MinSampleShading:     1.0,
	}

	depthStencil := &core.DepthStencilOptions{
		DepthTestEnable:  true,
		DepthWriteEnable: true,
		DepthCompareOp:   common.CompareLess,
	}

	colorBlend := &core.ColorBlendOptions{
		LogicOpEnabled: false,
		LogicOp:        common.LogicOpCopy,

		BlendConstants: [4]float32{0, 0, 0, 0},
		Attachments: []core.ColorBlendAttachment{
			{
				BlendEnabled: false,
				WriteMask:    common.ComponentRed | common.ComponentGreen | common.ComponentBlue | common.ComponentAlpha,
			},
		},
	}

	app.pipelineLayout, _, err = app.loader.CreatePipelineLayout(app.device, &core.PipelineLayoutOptions{
		SetLayouts: []core.DescriptorSetLayout{
			app.descriptorSetLayout,
		},
	})

	pipelines, _, err := app.loader.CreateGraphicsPipelines(app.device, []*core.GraphicsPipelineOptions{
		{
			ShaderStages: []*core.ShaderStage{
				vertStage,
				fragStage,
			},
			VertexInput:       vertexInput,
			InputAssembly:     inputAssembly,
			Viewport:          viewport,
			Rasterization:     rasterization,
			Multisample:       multisample,
			DepthStencil:      depthStencil,
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
		framebuffer, _, err := app.loader.CreateFrameBuffer(app.device, &core.FramebufferOptions{
			RenderPass: app.renderPass,
			Layers:     1,
			Attachments: []core.ImageView{
				imageView,
				app.depthImageView,
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

	pool, _, err := app.loader.CreateCommandPool(app.device, &core.CommandPoolOptions{
		GraphicsQueueFamily: indices.GraphicsFamily,
	})

	if err != nil {
		return err
	}
	app.commandPool = pool

	return nil
}

func (app *HelloTriangleApplication) createDepthResources() error {
	depthFormat, err := app.findDepthFormat()
	if err != nil {
		return err
	}

	app.depthImage, app.depthImageMemory, err = app.createImage(app.swapchainExtent.Width,
		app.swapchainExtent.Height,
		depthFormat,
		common.ImageTilingOptimal,
		common.ImageDepthStencilAttachment,
		core.MemoryDeviceLocal)
	if err != nil {
		return err
	}
	app.depthImageView, err = app.createImageView(app.depthImage, depthFormat, common.AspectDepth)
	return err
}

func (app *HelloTriangleApplication) findSupportedFormat(formats []common.DataFormat, tiling common.ImageTiling, features common.FormatFeatures) (common.DataFormat, error) {
	for _, format := range formats {
		props := app.physicalDevice.FormatProperties(format)

		if tiling == common.ImageTilingLinear && (props.LinearTilingFeatures&features) == features {
			return format, nil
		} else if tiling == common.ImageTilingOptimal && (props.OptimalTilingFeatures&features) == features {
			return format, nil
		}
	}

	return 0, errors.Newf("failed to find supported format for tiling %s, featureset %s", tiling, features)
}

func (app *HelloTriangleApplication) findDepthFormat() (common.DataFormat, error) {
	return app.findSupportedFormat([]common.DataFormat{common.FormatD32SignedFloat, common.FormatD32SignedFloatS8UnsignedInt, common.FormatD24UnsignedNormalizedS8UnsignedInt},
		common.ImageTilingOptimal,
		common.FormatFeatureDepthStencilAttachment)
}

func hasStencilComponent(format common.DataFormat) bool {
	return format == common.FormatD32SignedFloatS8UnsignedInt || format == common.FormatD24UnsignedNormalizedS8UnsignedInt
}

func (app *HelloTriangleApplication) createTextureImage() error {
	//Put image data into staging buffer
	imageBytes, err := fileSystem.ReadFile("images/viking_room.png")
	if err != nil {
		return err
	}

	decodedImage, err := png.Decode(bytes.NewBuffer(imageBytes))
	if err != nil {
		return err
	}
	imageBounds := decodedImage.Bounds()
	imageDims := imageBounds.Size()
	imageSize := imageDims.X * imageDims.Y * 4

	stagingBuffer, stagingMemory, err := app.createBuffer(imageSize, common.UsageTransferSrc, core.MemoryHostVisible|core.MemoryHostCoherent)
	if err != nil {
		return err
	}

	var pixelData []byte

	for y := imageBounds.Min.Y; y < imageBounds.Max.Y; y++ {
		for x := imageBounds.Min.X; x < imageBounds.Max.Y; x++ {
			r, g, b, a := decodedImage.At(x, y).RGBA()
			pixelData = append(pixelData, byte(r), byte(g), byte(b), byte(a))
		}
	}

	err = writeData(stagingMemory, 0, pixelData)
	if err != nil {
		return err
	}

	//Create final image
	app.textureImage, app.textureImageMemory, err = app.createImage(imageDims.X, imageDims.Y, common.FormatR8G8B8A8SRGB, common.ImageTilingOptimal, common.ImageTransferDest|common.ImageSampled, core.MemoryDeviceLocal)
	if err != nil {
		return err
	}

	// Copy staging to final
	err = app.transitionImageLayout(app.textureImage, common.FormatR8G8B8A8SRGB, common.LayoutUndefined, common.LayoutTransferDstOptimal)
	if err != nil {
		return err
	}
	err = app.copyBufferToImage(stagingBuffer, app.textureImage, imageDims.X, imageDims.Y)
	if err != nil {
		return err
	}
	err = app.transitionImageLayout(app.textureImage, common.FormatR8G8B8A8SRGB, common.LayoutTransferDstOptimal, common.LayoutShaderReadOnlyOptimal)
	if err != nil {
		return err
	}

	stagingBuffer.Destroy()
	app.device.FreeMemory(stagingMemory)

	return nil
}

func (app *HelloTriangleApplication) createTextureImageView() error {
	var err error
	app.textureImageView, err = app.createImageView(app.textureImage, common.FormatR8G8B8A8SRGB, common.AspectColor)
	return err
}

func (app *HelloTriangleApplication) createSampler() error {
	properties := app.physicalDevice.Properties()

	var err error
	app.textureSampler, _, err = app.loader.CreateSampler(app.device, &core.SamplerOptions{
		MagFilter:    common.FilterLinear,
		MinFilter:    common.FilterLinear,
		AddressModeU: common.AddressModeRepeat,
		AddressModeV: common.AddressModeRepeat,
		AddressModeW: common.AddressModeRepeat,

		AnisotropyEnable: true,
		MaxAnisotropy:    properties.Limits.MaxSamplerAnisotropy,

		BorderColor: common.BorderColorIntOpaqueBlack,

		MipmapMode: common.MipmapLinear,
	})

	return err
}

func (app *HelloTriangleApplication) createImageView(image core.Image, format common.DataFormat, aspect common.ImageAspectFlags) (core.ImageView, error) {
	imageView, _, err := app.loader.CreateImageView(app.device, &core.ImageViewOptions{
		Image:    image,
		ViewType: common.View2D,
		Format:   format,
		SubresourceRange: common.ImageSubresourceRange{
			AspectMask:     aspect,
			BaseMipLevel:   0,
			LevelCount:     1,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
	})
	return imageView, err
}

func (app *HelloTriangleApplication) createImage(width, height int, format common.DataFormat, tiling common.ImageTiling, usage common.ImageUsages, memoryProperties core.MemoryPropertyFlags) (core.Image, core.DeviceMemory, error) {
	image, _, err := app.loader.CreateImage(app.device, &core.ImageOptions{
		Type: common.ImageType2D,
		Extent: common.Extent3D{
			Width:  width,
			Height: height,
			Depth:  1,
		},
		MipLevels:     1,
		ArrayLayers:   1,
		Format:        format,
		Tiling:        tiling,
		InitialLayout: common.LayoutUndefined,
		Usage:         usage,
		SharingMode:   common.SharingExclusive,
		Samples:       common.Samples1,
	})
	if err != nil {
		return nil, nil, err
	}

	memReqs := image.MemoryRequirements()
	memoryIndex, err := app.findMemoryType(memReqs.MemoryType, memoryProperties)
	if err != nil {
		return nil, nil, err
	}

	imageMemory, _, err := app.device.AllocateMemory(&core.DeviceMemoryOptions{
		AllocationSize:  memReqs.Size,
		MemoryTypeIndex: memoryIndex,
	})

	_, err = image.BindImageMemory(imageMemory, 0)
	if err != nil {
		return nil, nil, err
	}

	return image, imageMemory, nil
}

func (app *HelloTriangleApplication) transitionImageLayout(image core.Image, format common.DataFormat, oldLayout common.ImageLayout, newLayout common.ImageLayout) error {
	buffer, err := app.beginSingleTimeCommands()
	if err != nil {
		return err
	}

	var sourceStage, destStage common.PipelineStages
	var sourceAccess, destAccess common.AccessFlags

	if oldLayout == common.LayoutUndefined && newLayout == common.LayoutTransferDstOptimal {
		sourceAccess = 0
		destAccess = common.AccessTransferWrite
		sourceStage = common.PipelineStageTopOfPipe
		destStage = common.PipelineStageTransfer
	} else if oldLayout == common.LayoutTransferDstOptimal && newLayout == common.LayoutShaderReadOnlyOptimal {
		sourceAccess = common.AccessTransferWrite
		destAccess = common.AccessShaderRead
		sourceStage = common.PipelineStageTransfer
		destStage = common.PipelineStageFragmentShader
	} else {
		return errors.Newf("unexpected layout transition: %s -> %s", oldLayout, newLayout)
	}

	err = buffer.CmdPipelineBarrier(sourceStage, destStage, 0, nil, nil, []*core.ImageMemoryBarrierOptions{
		{
			OldLayout:            oldLayout,
			NewLayout:            newLayout,
			SrcQueueFamilyIndex:  -1,
			DestQueueFamilyIndex: -1,
			Image:                image,
			SubresourceRange: common.ImageSubresourceRange{
				AspectMask:     common.AspectColor,
				BaseMipLevel:   0,
				LevelCount:     1,
				BaseArrayLayer: 0,
				LayerCount:     1,
			},
			SrcAccessMask:  sourceAccess,
			DestAccessMask: destAccess,
		},
	})
	if err != nil {
		return err
	}

	return app.endSingleTimeCommands(buffer)
}

func (app *HelloTriangleApplication) copyBufferToImage(buffer core.Buffer, image core.Image, width, height int) error {
	cmdBuffer, err := app.beginSingleTimeCommands()
	if err != nil {
		return err
	}

	err = cmdBuffer.CmdCopyBufferToImage(buffer, image, common.LayoutTransferDstOptimal, []*core.BufferImageCopy{
		{
			BufferOffset:      0,
			BufferRowLength:   0,
			BufferImageHeight: 0,

			ImageSubresource: common.ImageSubresourceLayers{
				AspectMask:     common.AspectColor,
				MipLevel:       0,
				BaseArrayLayer: 0,
				LayerCount:     1,
			},
			ImageOffset: common.Offset3D{X: 0, Y: 0, Z: 0},
			ImageExtent: common.Extent3D{Width: width, Height: height, Depth: 1},
		},
	})
	if err != nil {
		return err
	}

	return app.endSingleTimeCommands(cmdBuffer)
}

func writeData(memory core.DeviceMemory, offset int, data interface{}) error {
	bufferSize := binary.Size(data)

	memoryPtr, _, err := memory.MapMemory(offset, bufferSize, 0)
	if err != nil {
		return err
	}
	defer memory.UnmapMemory()

	dataBuffer := unsafe.Slice((*byte)(memoryPtr), bufferSize)

	buf := &bytes.Buffer{}
	err = binary.Write(buf, common.ByteOrder, data)
	if err != nil {
		return err
	}

	copy(dataBuffer, buf.Bytes())
	return nil
}

func (app *HelloTriangleApplication) addVertex(decoder *obj.Decoder, uniqueVertices map[int]uint32, face obj.Face, faceIndex int) {
	vertInd := face.Vertices[faceIndex]
	index, vertexExists := uniqueVertices[vertInd]

	if !vertexExists {
		vert := Vertex{Position: mgl32.Vec3{
			decoder.Vertices[vertInd*3],
			decoder.Vertices[vertInd*3+1],
			decoder.Vertices[vertInd*3+2],
		}, Color: mgl32.Vec3{1, 1, 1}}

		uvInd := face.Uvs[faceIndex]
		vert.TexCoord = mgl32.Vec2{
			decoder.Uvs[uvInd*2],
			1.0 - decoder.Uvs[uvInd*2+1],
		}

		index = uint32(len(app.vertices))
		app.vertices = append(app.vertices, vert)
		uniqueVertices[vertInd] = index
	}

	app.indices = append(app.indices, index)
}

func (app *HelloTriangleApplication) loadModel() error {
	meshFile, err := fileSystem.Open("meshes/viking_room.obj")
	if err != nil {
		return err
	}
	defer meshFile.Close()

	matFile, err := fileSystem.Open("meshes/viking_room.mtl")
	if err != nil {
		return err
	}
	defer matFile.Close()

	decoder, err := obj.DecodeReader(meshFile, matFile)
	if err != nil {
		return err
	}

	uniqueVertices := make(map[int]uint32)

	for _, decodedObj := range decoder.Objects {
		for _, face := range decodedObj.Faces {
			// We need to triangularize faces
			for i := 2; i < len(face.Vertices); i++ {
				app.addVertex(decoder, uniqueVertices, face, 0)
				app.addVertex(decoder, uniqueVertices, face, i-1)
				app.addVertex(decoder, uniqueVertices, face, i)
			}
		}
	}

	return nil
}

func (app *HelloTriangleApplication) createVertexBuffer() error {
	var err error
	bufferSize := binary.Size(app.vertices)

	stagingBuffer, stagingBufferMemory, err := app.createBuffer(bufferSize, common.UsageTransferSrc, core.MemoryHostVisible|core.MemoryHostCoherent)
	if stagingBuffer != nil {
		defer stagingBuffer.Destroy()
	}
	if stagingBufferMemory != nil {
		defer app.device.FreeMemory(stagingBufferMemory)
	}

	if err != nil {
		return err
	}

	err = writeData(stagingBufferMemory, 0, app.vertices)
	if err != nil {
		return err
	}

	app.vertexBuffer, app.vertexBufferMemory, err = app.createBuffer(bufferSize, common.UsageTransferDst|common.UsageVertexBuffer, core.MemoryDeviceLocal)
	if err != nil {
		return err
	}

	return app.copyBuffer(stagingBuffer, app.vertexBuffer, bufferSize)
}

func (app *HelloTriangleApplication) createIndexBuffer() error {
	bufferSize := binary.Size(app.indices)

	stagingBuffer, stagingBufferMemory, err := app.createBuffer(bufferSize, common.UsageTransferSrc, core.MemoryHostVisible|core.MemoryHostCoherent)
	if stagingBuffer != nil {
		defer stagingBuffer.Destroy()
	}
	if stagingBufferMemory != nil {
		defer app.device.FreeMemory(stagingBufferMemory)
	}

	if err != nil {
		return err
	}

	err = writeData(stagingBufferMemory, 0, app.indices)
	if err != nil {
		return err
	}

	app.indexBuffer, app.indexBufferMemory, err = app.createBuffer(bufferSize, common.UsageTransferDst|common.UsageIndexBuffer, core.MemoryDeviceLocal)
	if err != nil {
		return err
	}

	return app.copyBuffer(stagingBuffer, app.indexBuffer, bufferSize)
}

func (app *HelloTriangleApplication) createUniformBuffers() error {
	bufferSize := int(unsafe.Sizeof(UniformBufferObject{}))

	for i := 0; i < len(app.swapchainImages); i++ {
		buffer, memory, err := app.createBuffer(bufferSize, common.UsageUniformBuffer, core.MemoryHostVisible|core.MemoryHostCoherent)
		if err != nil {
			return err
		}

		app.uniformBuffers = append(app.uniformBuffers, buffer)
		app.uniformBuffersMemory = append(app.uniformBuffersMemory, memory)
	}

	return nil
}

func (app *HelloTriangleApplication) createDescriptorPool() error {
	var err error
	app.descriptorPool, _, err = app.loader.CreateDescriptorPool(app.device, &core.DescriptorPoolOptions{
		MaxSets: len(app.swapchainImages),
		PoolSizes: []core.PoolSize{
			{
				Type:  common.DescriptorUniformBuffer,
				Count: len(app.swapchainImages),
			},
			{
				Type:  common.DescriptorCombinedImageSampler,
				Count: len(app.swapchainImages),
			},
		},
	})
	return err
}

func (app *HelloTriangleApplication) createDescriptorSets() error {
	var allocLayouts []core.DescriptorSetLayout
	for i := 0; i < len(app.swapchainImages); i++ {
		allocLayouts = append(allocLayouts, app.descriptorSetLayout)
	}

	var err error
	app.descriptorSets, _, err = app.descriptorPool.AllocateDescriptorSets(&core.DescriptorSetOptions{
		AllocationLayouts: allocLayouts,
	})
	if err != nil {
		return err
	}

	for i := 0; i < len(app.swapchainImages); i++ {
		err = app.device.UpdateDescriptorSets([]core.WriteDescriptorSetOptions{
			{
				Destination:             app.descriptorSets[i],
				DestinationBinding:      0,
				DestinationArrayElement: 0,

				DescriptorType: common.DescriptorUniformBuffer,

				BufferInfo: []core.DescriptorBufferInfo{
					{
						Buffer: app.uniformBuffers[i],
						Offset: 0,
						Range:  int(unsafe.Sizeof(UniformBufferObject{})),
					},
				},
			},
			{
				Destination:             app.descriptorSets[i],
				DestinationBinding:      1,
				DestinationArrayElement: 0,

				DescriptorType: common.DescriptorCombinedImageSampler,

				ImageInfo: []core.DescriptorImageInfo{
					{
						ImageView:   app.textureImageView,
						Sampler:     app.textureSampler,
						ImageLayout: common.LayoutShaderReadOnlyOptimal,
					},
				},
			},
		}, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func (app *HelloTriangleApplication) createBuffer(size int, usage common.BufferUsages, properties core.MemoryPropertyFlags) (core.Buffer, core.DeviceMemory, error) {
	buffer, _, err := app.loader.CreateBuffer(app.device, &core.BufferOptions{
		BufferSize:  size,
		Usages:      usage,
		SharingMode: common.SharingExclusive,
	})
	if err != nil {
		return nil, nil, err
	}

	memRequirements := buffer.MemoryRequirements()
	memoryTypeIndex, err := app.findMemoryType(memRequirements.MemoryType, properties)
	if err != nil {
		return buffer, nil, err
	}

	memory, _, err := app.device.AllocateMemory(&core.DeviceMemoryOptions{
		AllocationSize:  memRequirements.Size,
		MemoryTypeIndex: memoryTypeIndex,
	})
	if err != nil {
		return buffer, nil, err
	}

	_, err = buffer.BindBufferMemory(memory, 0)
	return buffer, memory, err
}

func (app *HelloTriangleApplication) beginSingleTimeCommands() (core.CommandBuffer, error) {
	buffers, _, err := app.commandPool.AllocateCommandBuffers(&core.CommandBufferOptions{
		Level:       common.LevelPrimary,
		BufferCount: 1,
	})
	if err != nil {
		return nil, err
	}

	buffer := buffers[0]
	_, err = buffer.Begin(&core.BeginOptions{
		Flags: core.BeginInfoOneTimeSubmit,
	})
	return buffer, err
}

func (app *HelloTriangleApplication) endSingleTimeCommands(buffer core.CommandBuffer) error {
	_, err := buffer.End()
	if err != nil {
		return err
	}

	_, err = app.graphicsQueue.SubmitToQueue(nil, []*core.SubmitOptions{
		{
			CommandBuffers: []core.CommandBuffer{buffer},
		},
	})

	if err != nil {
		return err
	}

	_, err = app.graphicsQueue.WaitForIdle()
	if err != nil {
		return err
	}

	app.commandPool.FreeCommandBuffers([]core.CommandBuffer{buffer})
	return nil
}

func (app *HelloTriangleApplication) copyBuffer(srcBuffer core.Buffer, dstBuffer core.Buffer, size int) error {
	buffer, err := app.beginSingleTimeCommands()
	if err != nil {
		return err
	}

	err = buffer.CmdCopyBuffer(srcBuffer, dstBuffer, []core.BufferCopy{
		{
			SrcOffset: 0,
			DstOffset: 0,
			Size:      size,
		},
	})
	if err != nil {
		return err
	}

	return app.endSingleTimeCommands(buffer)
}

func (app *HelloTriangleApplication) findMemoryType(typeFilter uint32, properties core.MemoryPropertyFlags) (int, error) {
	memProperties := app.physicalDevice.MemoryProperties()
	for i, memoryType := range memProperties.MemoryTypes {
		typeBit := uint32(1 << i)

		if (typeFilter&typeBit) != 0 && (memoryType.Properties&properties) == properties {
			return i, nil
		}
	}

	return 0, stacktrace.NewError("failed to find any suitable memory type!")
}

func (app *HelloTriangleApplication) createCommandBuffers() error {

	buffers, _, err := app.commandPool.AllocateCommandBuffers(&core.CommandBufferOptions{
		Level:       common.LevelPrimary,
		BufferCount: len(app.swapchainImages),
	})
	if err != nil {
		return err
	}
	app.commandBuffers = buffers

	for bufferIdx, buffer := range buffers {
		_, err = buffer.Begin(&core.BeginOptions{})
		if err != nil {
			return err
		}

		err = buffer.CmdBeginRenderPass(core.ContentsInline,
			&core.RenderPassBeginOptions{
				RenderPass:  app.renderPass,
				Framebuffer: app.swapchainFramebuffers[bufferIdx],
				RenderArea: common.Rect2D{
					Offset: common.Offset2D{X: 0, Y: 0},
					Extent: app.swapchainExtent,
				},
				ClearValues: []core.ClearValue{
					core.ClearValueFloat{0, 0, 0, 1},
					core.ClearValueDepthStencil{Depth: 1.0, Stencil: 0},
				},
			})
		if err != nil {
			return err
		}

		buffer.CmdBindPipeline(common.BindGraphics, app.graphicsPipeline)
		buffer.CmdBindVertexBuffers(0, []core.Buffer{app.vertexBuffer}, []int{0})
		buffer.CmdBindIndexBuffer(app.indexBuffer, 0, common.IndexUInt32)
		buffer.CmdBindDescriptorSets(common.BindGraphics, app.pipelineLayout, 0, []core.DescriptorSet{
			app.descriptorSets[bufferIdx],
		}, nil)
		buffer.CmdDrawIndexed(len(app.indices), 1, 0, 0, 0)
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
		semaphore, _, err := app.loader.CreateSemaphore(app.device, &core.SemaphoreOptions{})
		if err != nil {
			return err
		}

		app.imageAvailableSemaphore = append(app.imageAvailableSemaphore, semaphore)

		semaphore, _, err = app.loader.CreateSemaphore(app.device, &core.SemaphoreOptions{})
		if err != nil {
			return err
		}

		app.renderFinishedSemaphore = append(app.renderFinishedSemaphore, semaphore)

		fence, _, err := app.loader.CreateFence(app.device, &core.FenceOptions{
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
	fences := []core.Fence{app.inFlightFence[app.currentFrame]}

	_, err := app.device.WaitForFences(true, common.NoTimeout, fences)
	if err != nil {
		return err
	}

	imageIndex, res, err := app.swapchain.AcquireNextImage(common.NoTimeout, app.imageAvailableSemaphore[app.currentFrame], nil)
	if res == core.VKErrorOutOfDateKHR {
		return app.recreateSwapChain()
	} else if err != nil {
		return err
	}

	if app.imagesInFlight[imageIndex] != nil {
		_, err := app.device.WaitForFences(true, common.NoTimeout, []core.Fence{app.imagesInFlight[imageIndex]})
		if err != nil {
			return err
		}
	}
	app.imagesInFlight[imageIndex] = app.inFlightFence[app.currentFrame]

	_, err = app.device.ResetFences(fences)
	if err != nil {
		return err
	}

	err = app.updateUniformBuffer(imageIndex)
	if err != nil {
		return err
	}

	_, err = app.graphicsQueue.SubmitToQueue(app.inFlightFence[app.currentFrame], []*core.SubmitOptions{
		{
			WaitSemaphores:   []core.Semaphore{app.imageAvailableSemaphore[app.currentFrame]},
			WaitDstStages:    []common.PipelineStages{common.PipelineStageColorAttachmentOutput},
			CommandBuffers:   []core.CommandBuffer{app.commandBuffers[imageIndex]},
			SignalSemaphores: []core.Semaphore{app.renderFinishedSemaphore[app.currentFrame]},
		},
	})
	if err != nil {
		return err
	}

	_, res, err = app.swapchain.PresentToQueue(app.presentQueue, &khr_swapchain.PresentOptions{
		WaitSemaphores: []core.Semaphore{app.renderFinishedSemaphore[app.currentFrame]},
		Swapchains:     []khr_swapchain.Swapchain{app.swapchain},
		ImageIndices:   []int{imageIndex},
	})
	if res == core.VKErrorOutOfDateKHR || res == core.VKSuboptimalKHR {
		return app.recreateSwapChain()
	} else if err != nil {
		return err
	}

	app.currentFrame = (app.currentFrame + 1) % MaxFramesInFlight

	return nil
}

func (app *HelloTriangleApplication) updateUniformBuffer(currentImage int) error {
	currentTime := hrtime.Now().Seconds()
	timePeriod := float32(math.Mod(currentTime, 4.0))

	ubo := UniformBufferObject{}
	ubo.Model = mgl32.HomogRotate3D(timePeriod*mgl32.DegToRad(90.0), mgl32.Vec3{0, 0, 1})
	ubo.View = mgl32.LookAt(2, 2, 2, 0, 0, 0, 0, 0, 1)
	aspectRatio := float32(app.swapchainExtent.Width) / float32(app.swapchainExtent.Height)

	near := 0.1
	far := 10.0
	fovy := mgl32.DegToRad(45)
	fmn, f := far-near, float32(1./math.Tan(float64(fovy)/2.0))

	ubo.Proj = mgl32.Mat4{float32(f / aspectRatio), 0, 0, 0, 0, float32(-f), 0, 0, 0, 0, float32(-far / fmn), -1, 0, 0, float32(-(far * near) / fmn), 0}

	err := writeData(app.uniformBuffersMemory[currentImage], 0, &ubo)
	return err
}

func (app *HelloTriangleApplication) chooseSwapSurfaceFormat(availableFormats []khr_surface.Format) khr_surface.Format {
	for _, format := range availableFormats {
		if format.Format == common.FormatB8G8R8A8SRGB && format.ColorSpace == khr_surface.ColorSpaceSRGBNonlinear {
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

func (app *HelloTriangleApplication) querySwapChainSupport(device core.PhysicalDevice) (SwapChainSupportDetails, error) {
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

func (app *HelloTriangleApplication) isDeviceSuitable(device core.PhysicalDevice) bool {
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

	features := device.Features()
	return indices.IsComplete() && extensionsSupported && swapChainAdequate && features.SamplerAnisotropy
}

func (app *HelloTriangleApplication) checkDeviceExtensionSupport(device core.PhysicalDevice) bool {
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

func (app *HelloTriangleApplication) findQueueFamilies(device core.PhysicalDevice) (QueueFamilyIndices, error) {
	indices := QueueFamilyIndices{}
	queueFamilies := device.QueueFamilyProperties()

	for queueFamilyIdx, queueFamily := range queueFamilies {
		if (queueFamily.Flags & common.QueueGraphics) != 0 {
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

func (app *HelloTriangleApplication) logDebug(msgType ext_debug_utils.MessageType, severity ext_debug_utils.MessageSeverity, data *ext_debug_utils.CallbackData) bool {
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

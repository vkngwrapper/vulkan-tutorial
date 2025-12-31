package main

import (
	"bytes"
	"embed"
	"encoding/binary"
	"image/png"
	"log"
	"math"
	"runtime"
	"unsafe"

	"github.com/g3n/engine/loader/obj"
	"github.com/loov/hrtime"
	"github.com/pkg/errors"
	"github.com/veandco/go-sdl2/sdl"
	"github.com/vkngwrapper/core/v3"
	"github.com/vkngwrapper/core/v3/common"
	"github.com/vkngwrapper/core/v3/core1_0"
	"github.com/vkngwrapper/extensions/v3/ext_debug_utils"
	"github.com/vkngwrapper/extensions/v3/khr_portability_enumeration"
	"github.com/vkngwrapper/extensions/v3/khr_portability_subset"
	"github.com/vkngwrapper/extensions/v3/khr_surface"
	"github.com/vkngwrapper/extensions/v3/khr_swapchain"
	vkng_sdl2 "github.com/vkngwrapper/integrations/sdl2/v3"
	vkngmath "github.com/vkngwrapper/math"
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
	Capabilities *khr_surface.SurfaceCapabilities
	Formats      []khr_surface.SurfaceFormat
	PresentModes []khr_surface.PresentMode
}

type Vertex struct {
	Position vkngmath.Vec3[float32]
	Color    vkngmath.Vec3[float32]
	TexCoord vkngmath.Vec2[float32]
}

type UniformBufferObject struct {
	Model vkngmath.Mat4x4[float32]
	View  vkngmath.Mat4x4[float32]
	Proj  vkngmath.Mat4x4[float32]
}

func getVertexBindingDescription() []core1_0.VertexInputBindingDescription {
	v := Vertex{}
	return []core1_0.VertexInputBindingDescription{
		{
			Binding:   0,
			Stride:    int(unsafe.Sizeof(v)),
			InputRate: core1_0.VertexInputRateVertex,
		},
	}
}

func getVertexAttributeDescriptions() []core1_0.VertexInputAttributeDescription {
	v := Vertex{}
	return []core1_0.VertexInputAttributeDescription{
		{
			Binding:  0,
			Location: 0,
			Format:   core1_0.FormatR32G32B32SignedFloat,
			Offset:   int(unsafe.Offsetof(v.Position)),
		},
		{
			Binding:  0,
			Location: 1,
			Format:   core1_0.FormatR32G32B32SignedFloat,
			Offset:   int(unsafe.Offsetof(v.Color)),
		},
		{
			Binding:  0,
			Location: 2,
			Format:   core1_0.FormatR32G32SignedFloat,
			Offset:   int(unsafe.Offsetof(v.TexCoord)),
		},
	}
}

type HelloTriangleApplication struct {
	globalDriver     core1_0.GlobalDriver
	instanceDriver   core1_0.CoreInstanceDriver
	deviceDriver     core1_0.DeviceDriver
	debugExtension   ext_debug_utils.ExtensionDriver
	surfaceExtension khr_surface.ExtensionDriver
	window           *sdl.Window

	debugMessenger ext_debug_utils.DebugUtilsMessenger
	surface        khr_surface.Surface
	physicalDevice core1_0.PhysicalDevice

	graphicsQueue core1_0.Queue
	presentQueue  core1_0.Queue

	swapchainExtension    khr_swapchain.ExtensionDriver
	swapchain             khr_swapchain.Swapchain
	swapchainImages       []core1_0.Image
	swapchainImageFormat  core1_0.Format
	swapchainExtent       core1_0.Extent2D
	swapchainImageViews   []core1_0.ImageView
	swapchainFramebuffers []core1_0.Framebuffer

	renderPass          core1_0.RenderPass
	descriptorPool      core1_0.DescriptorPool
	descriptorSets      []core1_0.DescriptorSet
	descriptorSetLayout core1_0.DescriptorSetLayout
	pipelineLayout      core1_0.PipelineLayout
	graphicsPipeline    core1_0.Pipeline

	commandPool    core1_0.CommandPool
	commandBuffers []core1_0.CommandBuffer

	imageAvailableSemaphore []core1_0.Semaphore
	renderFinishedSemaphore []core1_0.Semaphore
	inFlightFence           []core1_0.Fence
	imagesInFlight          []core1_0.Fence
	currentFrame            int
	frameStart              float64

	vertices           []Vertex
	indices            []uint32
	vertexBuffer       core1_0.Buffer
	vertexBufferMemory core1_0.DeviceMemory
	indexBuffer        core1_0.Buffer
	indexBufferMemory  core1_0.DeviceMemory

	uniformBuffers       []core1_0.Buffer
	uniformBuffersMemory []core1_0.DeviceMemory

	mipLevels          int
	textureImage       core1_0.Image
	textureImageMemory core1_0.DeviceMemory
	textureImageView   core1_0.ImageView
	textureSampler     core1_0.Sampler

	depthImage       core1_0.Image
	depthImageMemory core1_0.DeviceMemory
	depthImageView   core1_0.ImageView

	msaaSamples      core1_0.SampleCountFlags
	colorImage       core1_0.Image
	colorImageMemory core1_0.DeviceMemory
	colorImageView   core1_0.ImageView
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

	app.globalDriver, err = core.CreateDriverFromProcAddr(sdl.VulkanGetVkGetInstanceProcAddr())
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

	err = app.createColorResources()
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
	for {
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
						err := app.recreateSwapChain()
						if err != nil {
							return err
						}
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

	_, err := app.deviceDriver.DeviceWaitIdle()
	return err
}

func (app *HelloTriangleApplication) cleanupSwapChain() {
	if app.colorImageView.Initialized() {
		app.deviceDriver.DestroyImageView(app.colorImageView, nil)
		app.colorImageView = core1_0.ImageView{}
	}

	if app.colorImage.Initialized() {
		app.deviceDriver.DestroyImage(app.colorImage, nil)
		app.colorImage = core1_0.Image{}
	}

	if app.colorImageMemory.Initialized() {
		app.deviceDriver.FreeMemory(app.colorImageMemory, nil)
		app.colorImageMemory = core1_0.DeviceMemory{}
	}

	if app.depthImageView.Initialized() {
		app.deviceDriver.DestroyImageView(app.depthImageView, nil)
		app.depthImageView = core1_0.ImageView{}
	}

	if app.depthImage.Initialized() {
		app.deviceDriver.DestroyImage(app.depthImage, nil)
		app.depthImage = core1_0.Image{}
	}

	if app.depthImageMemory.Initialized() {
		app.deviceDriver.FreeMemory(app.depthImageMemory, nil)
		app.depthImageMemory = core1_0.DeviceMemory{}
	}

	for _, framebuffer := range app.swapchainFramebuffers {
		app.deviceDriver.DestroyFramebuffer(framebuffer, nil)
	}
	app.swapchainFramebuffers = []core1_0.Framebuffer{}

	if len(app.commandBuffers) > 0 {
		app.deviceDriver.FreeCommandBuffers(app.commandBuffers...)
		app.commandBuffers = []core1_0.CommandBuffer{}
	}

	if app.graphicsPipeline.Initialized() {
		app.deviceDriver.DestroyPipeline(app.graphicsPipeline, nil)
		app.graphicsPipeline = core1_0.Pipeline{}
	}

	if app.pipelineLayout.Initialized() {
		app.deviceDriver.DestroyPipelineLayout(app.pipelineLayout, nil)
		app.pipelineLayout = core1_0.PipelineLayout{}
	}

	if app.renderPass.Initialized() {
		app.deviceDriver.DestroyRenderPass(app.renderPass, nil)
		app.renderPass = core1_0.RenderPass{}
	}

	for _, imageView := range app.swapchainImageViews {
		app.deviceDriver.DestroyImageView(imageView, nil)
	}
	app.swapchainImageViews = []core1_0.ImageView{}

	if app.swapchain.Initialized() {
		app.swapchainExtension.DestroySwapchain(app.swapchain, nil)
		app.swapchain = khr_swapchain.Swapchain{}
	}

	for i := 0; i < len(app.uniformBuffers); i++ {
		app.deviceDriver.DestroyBuffer(app.uniformBuffers[i], nil)
	}
	app.uniformBuffers = app.uniformBuffers[:0]

	for i := 0; i < len(app.uniformBuffersMemory); i++ {
		app.deviceDriver.FreeMemory(app.uniformBuffersMemory[i], nil)
	}
	app.uniformBuffersMemory = app.uniformBuffersMemory[:0]

	if app.descriptorPool.Initialized() {
		app.deviceDriver.DestroyDescriptorPool(app.descriptorPool, nil)
	}
}

func (app *HelloTriangleApplication) cleanup() {
	app.cleanupSwapChain()

	if app.textureSampler.Initialized() {
		app.deviceDriver.DestroySampler(app.textureSampler, nil)
	}

	if app.textureImageView.Initialized() {
		app.deviceDriver.DestroyImageView(app.textureImageView, nil)
	}

	if app.textureImage.Initialized() {
		app.deviceDriver.DestroyImage(app.textureImage, nil)
	}

	if app.textureImageMemory.Initialized() {
		app.deviceDriver.FreeMemory(app.textureImageMemory, nil)
	}

	if app.descriptorSetLayout.Initialized() {
		app.deviceDriver.DestroyDescriptorSetLayout(app.descriptorSetLayout, nil)
	}

	if app.indexBuffer.Initialized() {
		app.deviceDriver.DestroyBuffer(app.indexBuffer, nil)
	}

	if app.indexBufferMemory.Initialized() {
		app.deviceDriver.FreeMemory(app.indexBufferMemory, nil)
	}

	if app.vertexBuffer.Initialized() {
		app.deviceDriver.DestroyBuffer(app.vertexBuffer, nil)
	}

	if app.vertexBufferMemory.Initialized() {
		app.deviceDriver.FreeMemory(app.vertexBufferMemory, nil)
	}

	for _, fence := range app.inFlightFence {
		app.deviceDriver.DestroyFence(fence, nil)
	}

	for _, semaphore := range app.renderFinishedSemaphore {
		app.deviceDriver.DestroySemaphore(semaphore, nil)
	}

	for _, semaphore := range app.imageAvailableSemaphore {
		app.deviceDriver.DestroySemaphore(semaphore, nil)
	}

	if app.commandPool.Initialized() {
		app.deviceDriver.DestroyCommandPool(app.commandPool, nil)
	}

	if app.deviceDriver != nil {
		app.deviceDriver.DestroyDevice(nil)
	}

	if app.debugMessenger.Initialized() {
		app.debugExtension.DestroyDebugUtilsMessenger(app.debugMessenger, nil)
	}

	if app.surface.Initialized() {
		app.surfaceExtension.DestroySurface(app.surface, nil)
	}

	if app.instanceDriver != nil {
		app.instanceDriver.DestroyInstance(nil)
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

	_, err := app.deviceDriver.DeviceWaitIdle()
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

	err = app.createColorResources()
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

	app.imagesInFlight = []core1_0.Fence{}
	for i := 0; i < len(app.swapchainImages); i++ {
		app.imagesInFlight = append(app.imagesInFlight, core1_0.Fence{})
	}

	return nil
}

func (app *HelloTriangleApplication) createInstance() error {
	instanceOptions := core1_0.InstanceCreateInfo{
		ApplicationName:    "Hello Triangle",
		ApplicationVersion: common.CreateVersion(1, 0, 0),
		EngineName:         "No Engine",
		EngineVersion:      common.CreateVersion(1, 0, 0),
		APIVersion:         common.Vulkan1_2,
	}

	// Add extensions
	sdlExtensions := app.window.VulkanGetInstanceExtensions()
	extensions, _, err := app.globalDriver.AvailableExtensions()
	if err != nil {
		return err
	}

	for _, ext := range sdlExtensions {
		_, hasExt := extensions[ext]
		if !hasExt {
			return errors.Errorf("createinstance: cannot initialize sdl: missing extension %s", ext)
		}
		instanceOptions.EnabledExtensionNames = append(instanceOptions.EnabledExtensionNames, ext)
	}

	if enableValidationLayers {
		instanceOptions.EnabledExtensionNames = append(instanceOptions.EnabledExtensionNames, ext_debug_utils.ExtensionName)
	}

	_, enumerationSupported := extensions[khr_portability_enumeration.ExtensionName]
	if enumerationSupported {
		instanceOptions.EnabledExtensionNames = append(instanceOptions.EnabledExtensionNames, khr_portability_enumeration.ExtensionName)
		instanceOptions.Flags |= khr_portability_enumeration.InstanceCreateEnumeratePortability
	}

	// Add layers
	layers, _, err := app.globalDriver.AvailableLayers()
	if err != nil {
		return err
	}

	if enableValidationLayers {
		for _, layer := range validationLayers {
			_, hasValidation := layers[layer]
			if !hasValidation {
				return errors.Errorf("createInstance: cannot add validation- layer %s not available- install LunarG Vulkan SDK", layer)
			}
			instanceOptions.EnabledLayerNames = append(instanceOptions.EnabledLayerNames, layer)
		}

		// Add debug messenger
		instanceOptions.Next = app.debugMessengerOptions()
	}

	app.instanceDriver, _, err = app.globalDriver.CreateInstance(nil, instanceOptions)
	if err != nil {
		return err
	}

	return nil
}

func (app *HelloTriangleApplication) debugMessengerOptions() ext_debug_utils.DebugUtilsMessengerCreateInfo {
	return ext_debug_utils.DebugUtilsMessengerCreateInfo{
		MessageSeverity: ext_debug_utils.SeverityError | ext_debug_utils.SeverityWarning,
		MessageType:     ext_debug_utils.TypeGeneral | ext_debug_utils.TypeValidation | ext_debug_utils.TypePerformance,
		UserCallback:    app.logDebug,
	}
}

func (app *HelloTriangleApplication) setupDebugMessenger() error {
	if !enableValidationLayers {
		return nil
	}

	var err error
	app.debugExtension = ext_debug_utils.CreateExtensionDriverFromCoreDriver(app.instanceDriver)
	app.debugMessenger, _, err = app.debugExtension.CreateDebugUtilsMessenger(nil, app.debugMessengerOptions())
	if err != nil {
		return err
	}

	return nil
}

func (app *HelloTriangleApplication) createSurface() error {
	app.surfaceExtension = khr_surface.CreateExtensionDriverFromCoreDriver(app.instanceDriver)

	surface, err := vkng_sdl2.CreateSurface(app.instanceDriver.Instance(), app.surfaceExtension, app.window)
	if err != nil {
		return err
	}

	app.surface = surface
	return nil
}

func (app *HelloTriangleApplication) pickPhysicalDevice() error {
	physicalDevices, _, err := app.instanceDriver.EnumeratePhysicalDevices()
	if err != nil {
		return err
	}

	for _, device := range physicalDevices {
		if app.isDeviceSuitable(device) {
			app.physicalDevice = device
			app.msaaSamples, err = app.getMaxUsableSampleCount()
			if err != nil {
				return err
			}
			break
		}
	}

	if !app.physicalDevice.Initialized() {
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

	var queueFamilyOptions []core1_0.DeviceQueueCreateInfo
	queuePriority := float32(1.0)
	for _, queueFamily := range uniqueQueueFamilies {
		queueFamilyOptions = append(queueFamilyOptions, core1_0.DeviceQueueCreateInfo{
			QueueFamilyIndex: queueFamily,
			QueuePriorities:  []float32{queuePriority},
		})
	}

	var extensionNames []string
	extensionNames = append(extensionNames, deviceExtensions...)

	// Makes this example compatible with vulkan portability, necessary to run on mobile & mac
	extensions, _, err := app.instanceDriver.EnumerateDeviceExtensionProperties(app.physicalDevice)
	if err != nil {
		return err
	}

	_, supported := extensions[khr_portability_subset.ExtensionName]
	if supported {
		extensionNames = append(extensionNames, khr_portability_subset.ExtensionName)
	}

	app.deviceDriver, _, err = app.instanceDriver.CreateDevice(app.physicalDevice, nil, core1_0.DeviceCreateInfo{
		QueueCreateInfos: queueFamilyOptions,
		EnabledFeatures: &core1_0.PhysicalDeviceFeatures{
			SamplerAnisotropy: true,
		},
		EnabledExtensionNames: extensionNames,
	})
	if err != nil {
		return err
	}

	app.graphicsQueue = app.deviceDriver.GetQueue(*indices.GraphicsFamily, 0)
	app.presentQueue = app.deviceDriver.GetQueue(*indices.PresentFamily, 0)
	return nil
}

func (app *HelloTriangleApplication) createSwapchain() error {
	app.swapchainExtension = khr_swapchain.CreateExtensionDriverFromCoreDriver(app.deviceDriver)

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

	sharingMode := core1_0.SharingModeExclusive
	var queueFamilyIndices []int

	indices, err := app.findQueueFamilies(app.physicalDevice)
	if err != nil {
		return err
	}

	if *indices.GraphicsFamily != *indices.PresentFamily {
		sharingMode = core1_0.SharingModeConcurrent
		queueFamilyIndices = append(queueFamilyIndices, *indices.GraphicsFamily, *indices.PresentFamily)
	}

	swapchain, _, err := app.swapchainExtension.CreateSwapchain(nil, khr_swapchain.SwapchainCreateInfo{
		Surface: app.surface,

		MinImageCount:    imageCount,
		ImageFormat:      surfaceFormat.Format,
		ImageColorSpace:  surfaceFormat.ColorSpace,
		ImageExtent:      extent,
		ImageArrayLayers: 1,
		ImageUsage:       core1_0.ImageUsageColorAttachment,

		ImageSharingMode:   sharingMode,
		QueueFamilyIndices: queueFamilyIndices,

		PreTransform:   swapchainSupport.Capabilities.CurrentTransform,
		CompositeAlpha: khr_surface.CompositeAlphaOpaque,
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
	images, _, err := app.swapchainExtension.GetSwapchainImages(app.swapchain)
	if err != nil {
		return err
	}
	app.swapchainImages = images

	var imageViews []core1_0.ImageView
	for _, image := range images {
		view, err := app.createImageView(image, app.swapchainImageFormat, core1_0.ImageAspectColor, 1)
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

	renderPass, _, err := app.deviceDriver.CreateRenderPass(nil, core1_0.RenderPassCreateInfo{
		Attachments: []core1_0.AttachmentDescription{
			{
				Format:         app.swapchainImageFormat,
				Samples:        app.msaaSamples,
				LoadOp:         core1_0.AttachmentLoadOpClear,
				StoreOp:        core1_0.AttachmentStoreOpStore,
				StencilLoadOp:  core1_0.AttachmentLoadOpDontCare,
				StencilStoreOp: core1_0.AttachmentStoreOpDontCare,
				InitialLayout:  core1_0.ImageLayoutUndefined,
				FinalLayout:    core1_0.ImageLayoutColorAttachmentOptimal,
			},
			{
				Format:         depthFormat,
				Samples:        app.msaaSamples,
				LoadOp:         core1_0.AttachmentLoadOpClear,
				StoreOp:        core1_0.AttachmentStoreOpDontCare,
				StencilLoadOp:  core1_0.AttachmentLoadOpDontCare,
				StencilStoreOp: core1_0.AttachmentStoreOpDontCare,
				InitialLayout:  core1_0.ImageLayoutUndefined,
				FinalLayout:    core1_0.ImageLayoutDepthStencilAttachmentOptimal,
			},
			{
				Format:         app.swapchainImageFormat,
				Samples:        core1_0.Samples1,
				LoadOp:         core1_0.AttachmentLoadOpDontCare,
				StoreOp:        core1_0.AttachmentStoreOpStore,
				StencilLoadOp:  core1_0.AttachmentLoadOpDontCare,
				StencilStoreOp: core1_0.AttachmentStoreOpDontCare,
				InitialLayout:  core1_0.ImageLayoutUndefined,
				FinalLayout:    khr_swapchain.ImageLayoutPresentSrc,
			},
		},
		Subpasses: []core1_0.SubpassDescription{
			{
				PipelineBindPoint: core1_0.PipelineBindPointGraphics,
				ColorAttachments: []core1_0.AttachmentReference{
					{
						Attachment: 0,
						Layout:     core1_0.ImageLayoutColorAttachmentOptimal,
					},
				},
				ResolveAttachments: []core1_0.AttachmentReference{
					{
						Attachment: 2,
						Layout:     core1_0.ImageLayoutColorAttachmentOptimal,
					},
				},
				DepthStencilAttachment: &core1_0.AttachmentReference{
					Attachment: 1,
					Layout:     core1_0.ImageLayoutDepthStencilAttachmentOptimal,
				},
			},
		},
		SubpassDependencies: []core1_0.SubpassDependency{
			{
				SrcSubpass: core1_0.SubpassExternal,
				DstSubpass: 0,

				SrcStageMask:  core1_0.PipelineStageColorAttachmentOutput | core1_0.PipelineStageEarlyFragmentTests,
				SrcAccessMask: 0,

				DstStageMask:  core1_0.PipelineStageColorAttachmentOutput | core1_0.PipelineStageEarlyFragmentTests,
				DstAccessMask: core1_0.AccessColorAttachmentWrite | core1_0.AccessDepthStencilAttachmentWrite,
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
	app.descriptorSetLayout, _, err = app.deviceDriver.CreateDescriptorSetLayout(nil, core1_0.DescriptorSetLayoutCreateInfo{
		Bindings: []core1_0.DescriptorSetLayoutBinding{
			{
				Binding:         0,
				DescriptorType:  core1_0.DescriptorTypeUniformBuffer,
				DescriptorCount: 1,

				StageFlags: core1_0.StageVertex,
			},
			{
				Binding:         1,
				DescriptorType:  core1_0.DescriptorTypeCombinedImageSampler,
				DescriptorCount: 1,

				StageFlags: core1_0.StageFragment,
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

	vertShader, _, err := app.deviceDriver.CreateShaderModule(nil, core1_0.ShaderModuleCreateInfo{
		Code: bytesToBytecode(vertShaderBytes),
	})
	if err != nil {
		return err
	}
	defer app.deviceDriver.DestroyShaderModule(vertShader, nil)

	// Load fragment shader
	fragShaderBytes, err := fileSystem.ReadFile("shaders/frag.spv")
	if err != nil {
		return err
	}

	fragShader, _, err := app.deviceDriver.CreateShaderModule(nil, core1_0.ShaderModuleCreateInfo{
		Code: bytesToBytecode(fragShaderBytes),
	})
	if err != nil {
		return err
	}
	defer app.deviceDriver.DestroyShaderModule(fragShader, nil)

	vertexInput := &core1_0.PipelineVertexInputStateCreateInfo{
		VertexBindingDescriptions:   getVertexBindingDescription(),
		VertexAttributeDescriptions: getVertexAttributeDescriptions(),
	}

	inputAssembly := &core1_0.PipelineInputAssemblyStateCreateInfo{
		Topology:               core1_0.PrimitiveTopologyTriangleList,
		PrimitiveRestartEnable: false,
	}

	vertStage := core1_0.PipelineShaderStageCreateInfo{
		Stage:  core1_0.StageVertex,
		Module: vertShader,
		Name:   "main",
	}

	fragStage := core1_0.PipelineShaderStageCreateInfo{
		Stage:  core1_0.StageFragment,
		Module: fragShader,
		Name:   "main",
	}

	viewport := &core1_0.PipelineViewportStateCreateInfo{
		Viewports: []core1_0.Viewport{
			{
				X:        0,
				Y:        0,
				Width:    float32(app.swapchainExtent.Width),
				Height:   float32(app.swapchainExtent.Height),
				MinDepth: 0,
				MaxDepth: 1,
			},
		},
		Scissors: []core1_0.Rect2D{
			{
				Offset: core1_0.Offset2D{X: 0, Y: 0},
				Extent: app.swapchainExtent,
			},
		},
	}

	rasterization := &core1_0.PipelineRasterizationStateCreateInfo{
		DepthClampEnable:        false,
		RasterizerDiscardEnable: false,

		PolygonMode: core1_0.PolygonModeFill,
		CullMode:    core1_0.CullModeBack,
		FrontFace:   core1_0.FrontFaceCounterClockwise,

		DepthBiasEnable: false,

		LineWidth: 1.0,
	}

	multisample := &core1_0.PipelineMultisampleStateCreateInfo{
		SampleShadingEnable:  false,
		RasterizationSamples: app.msaaSamples,
		MinSampleShading:     1.0,
	}

	depthStencil := &core1_0.PipelineDepthStencilStateCreateInfo{
		DepthTestEnable:  true,
		DepthWriteEnable: true,
		DepthCompareOp:   core1_0.CompareOpLess,
	}

	colorBlend := &core1_0.PipelineColorBlendStateCreateInfo{
		LogicOpEnabled: false,
		LogicOp:        core1_0.LogicOpCopy,

		BlendConstants: [4]float32{0, 0, 0, 0},
		Attachments: []core1_0.PipelineColorBlendAttachmentState{
			{
				BlendEnabled:   false,
				ColorWriteMask: core1_0.ColorComponentRed | core1_0.ColorComponentGreen | core1_0.ColorComponentBlue | core1_0.ColorComponentAlpha,
			},
		},
	}

	app.pipelineLayout, _, err = app.deviceDriver.CreatePipelineLayout(nil, core1_0.PipelineLayoutCreateInfo{
		SetLayouts: []core1_0.DescriptorSetLayout{
			app.descriptorSetLayout,
		},
	})
	if err != nil {
		return err
	}

	pipelines, _, err := app.deviceDriver.CreateGraphicsPipelines(nil, nil,
		core1_0.GraphicsPipelineCreateInfo{
			Stages: []core1_0.PipelineShaderStageCreateInfo{
				vertStage,
				fragStage,
			},
			VertexInputState:   vertexInput,
			InputAssemblyState: inputAssembly,
			ViewportState:      viewport,
			RasterizationState: rasterization,
			MultisampleState:   multisample,
			DepthStencilState:  depthStencil,
			ColorBlendState:    colorBlend,
			Layout:             app.pipelineLayout,
			RenderPass:         app.renderPass,
			Subpass:            0,
			BasePipelineIndex:  -1,
		},
	)
	if err != nil {
		return err
	}
	app.graphicsPipeline = pipelines[0]

	return nil
}

func (app *HelloTriangleApplication) createFramebuffers() error {
	for _, imageView := range app.swapchainImageViews {
		framebuffer, _, err := app.deviceDriver.CreateFramebuffer(nil, core1_0.FramebufferCreateInfo{
			RenderPass: app.renderPass,
			Layers:     1,
			Attachments: []core1_0.ImageView{
				app.colorImageView,
				app.depthImageView,
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

	pool, _, err := app.deviceDriver.CreateCommandPool(nil, core1_0.CommandPoolCreateInfo{
		QueueFamilyIndex: *indices.GraphicsFamily,
	})

	if err != nil {
		return err
	}
	app.commandPool = pool

	return nil
}

func (app *HelloTriangleApplication) createColorResources() error {
	var err error
	app.colorImage, app.colorImageMemory, err = app.createImage(
		app.swapchainExtent.Width,
		app.swapchainExtent.Height,
		1,
		app.msaaSamples,
		app.swapchainImageFormat,
		core1_0.ImageTilingOptimal,
		core1_0.ImageUsageTransientAttachment|core1_0.ImageUsageColorAttachment,
		core1_0.MemoryPropertyDeviceLocal)
	if err != nil {
		return err
	}

	app.colorImageView, err = app.createImageView(
		app.colorImage,
		app.swapchainImageFormat,
		core1_0.ImageAspectColor,
		1)
	return err
}

func (app *HelloTriangleApplication) createDepthResources() error {
	depthFormat, err := app.findDepthFormat()
	if err != nil {
		return err
	}

	app.depthImage, app.depthImageMemory, err = app.createImage(app.swapchainExtent.Width,
		app.swapchainExtent.Height,
		1,
		app.msaaSamples,
		depthFormat,
		core1_0.ImageTilingOptimal,
		core1_0.ImageUsageDepthStencilAttachment,
		core1_0.MemoryPropertyDeviceLocal)
	if err != nil {
		return err
	}
	app.depthImageView, err = app.createImageView(app.depthImage, depthFormat, core1_0.ImageAspectDepth, 1)
	return err
}

func (app *HelloTriangleApplication) findSupportedFormat(formats []core1_0.Format, tiling core1_0.ImageTiling, features core1_0.FormatFeatureFlags) (core1_0.Format, error) {
	for _, format := range formats {
		props := app.instanceDriver.GetPhysicalDeviceFormatProperties(app.physicalDevice, format)

		if tiling == core1_0.ImageTilingLinear && (props.LinearTilingFeatures&features) == features {
			return format, nil
		} else if tiling == core1_0.ImageTilingOptimal && (props.OptimalTilingFeatures&features) == features {
			return format, nil
		}
	}

	return 0, errors.Errorf("failed to find supported format for tiling %s, featureset %s", tiling, features)
}

func (app *HelloTriangleApplication) findDepthFormat() (core1_0.Format, error) {
	return app.findSupportedFormat([]core1_0.Format{core1_0.FormatD32SignedFloat, core1_0.FormatD32SignedFloatS8UnsignedInt, core1_0.FormatD24UnsignedNormalizedS8UnsignedInt},
		core1_0.ImageTilingOptimal,
		core1_0.FormatFeatureDepthStencilAttachment)
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

	app.mipLevels = int(math.Log2(math.Max(float64(imageDims.X), float64(imageDims.Y))))

	stagingBuffer, stagingMemory, err := app.createBuffer(imageSize, core1_0.BufferUsageTransferSrc, core1_0.MemoryPropertyHostVisible|core1_0.MemoryPropertyHostCoherent)
	if err != nil {
		return err
	}

	defer app.deviceDriver.DestroyBuffer(stagingBuffer, nil)
	defer app.deviceDriver.FreeMemory(stagingMemory, nil)

	var pixelData []byte

	for y := imageBounds.Min.Y; y < imageBounds.Max.Y; y++ {
		for x := imageBounds.Min.X; x < imageBounds.Max.Y; x++ {
			r, g, b, a := decodedImage.At(x, y).RGBA()
			pixelData = append(pixelData, byte(r), byte(g), byte(b), byte(a))
		}
	}

	err = writeData(app.deviceDriver, stagingMemory, 0, pixelData)
	if err != nil {
		return err
	}

	//Create final image
	app.textureImage, app.textureImageMemory, err = app.createImage(imageDims.X,
		imageDims.Y,
		app.mipLevels,
		core1_0.Samples1,
		core1_0.FormatR8G8B8A8SRGB,
		core1_0.ImageTilingOptimal,
		core1_0.ImageUsageTransferSrc|core1_0.ImageUsageTransferDst|core1_0.ImageUsageSampled,
		core1_0.MemoryPropertyDeviceLocal)
	if err != nil {
		return err
	}

	// Copy staging to final
	err = app.transitionImageLayout(app.textureImage, core1_0.FormatR8G8B8A8SRGB, core1_0.ImageLayoutUndefined, core1_0.ImageLayoutTransferDstOptimal, app.mipLevels)
	if err != nil {
		return err
	}
	err = app.copyBufferToImage(stagingBuffer, app.textureImage, imageDims.X, imageDims.Y)
	if err != nil {
		return err
	}

	return app.generateMipmaps(app.textureImage, core1_0.FormatR8G8B8A8SRGB, imageDims.X, imageDims.Y, app.mipLevels)
}

func (app *HelloTriangleApplication) generateMipmaps(image core1_0.Image, imageFormat core1_0.Format, width, height int, mipLevels int) error {

	properties := app.instanceDriver.GetPhysicalDeviceFormatProperties(app.physicalDevice, imageFormat)

	if (properties.OptimalTilingFeatures & core1_0.FormatFeatureSampledImageFilterLinear) == 0 {
		return errors.Errorf("texture image format %s does not support linear blitting", imageFormat)
	}

	commandBuffer, err := app.beginSingleTimeCommands()
	if err != nil {
		return err
	}

	barrier := core1_0.ImageMemoryBarrier{
		Image:               image,
		SrcQueueFamilyIndex: -1,
		DstQueueFamilyIndex: -1,
		SubresourceRange: core1_0.ImageSubresourceRange{
			AspectMask:     core1_0.ImageAspectColor,
			BaseArrayLayer: 0,
			LayerCount:     1,
			LevelCount:     1,
		},
	}

	mipWidth := width
	mipHeight := height
	for i := 1; i < mipLevels; i++ {
		barrier.SubresourceRange.BaseMipLevel = i - 1
		barrier.OldLayout = core1_0.ImageLayoutTransferDstOptimal
		barrier.NewLayout = core1_0.ImageLayoutTransferSrcOptimal
		barrier.SrcAccessMask = core1_0.AccessTransferWrite
		barrier.DstAccessMask = core1_0.AccessTransferRead

		err = app.deviceDriver.CmdPipelineBarrier(commandBuffer, core1_0.PipelineStageTransfer, core1_0.PipelineStageTransfer, 0, nil, nil, []core1_0.ImageMemoryBarrier{barrier})
		if err != nil {
			return err
		}

		nextMipWidth := mipWidth
		nextMipHeight := mipHeight

		if nextMipWidth > 1 {
			nextMipWidth /= 2
		}
		if nextMipHeight > 1 {
			nextMipHeight /= 2
		}
		err = app.deviceDriver.CmdBlitImage(commandBuffer, image, core1_0.ImageLayoutTransferSrcOptimal, image, core1_0.ImageLayoutTransferDstOptimal, []core1_0.ImageBlit{
			{
				SrcSubresource: core1_0.ImageSubresourceLayers{
					AspectMask:     core1_0.ImageAspectColor,
					MipLevel:       i - 1,
					BaseArrayLayer: 0,
					LayerCount:     1,
				},
				SrcOffsets: [2]core1_0.Offset3D{
					{X: 0, Y: 0, Z: 0},
					{X: mipWidth, Y: mipHeight, Z: 1},
				},

				DstSubresource: core1_0.ImageSubresourceLayers{
					AspectMask:     core1_0.ImageAspectColor,
					MipLevel:       i,
					BaseArrayLayer: 0,
					LayerCount:     1,
				},
				DstOffsets: [2]core1_0.Offset3D{
					{X: 0, Y: 0, Z: 0},
					{X: nextMipWidth, Y: nextMipHeight, Z: 1},
				},
			},
		}, core1_0.FilterLinear)
		if err != nil {
			return err
		}

		barrier.OldLayout = core1_0.ImageLayoutTransferSrcOptimal
		barrier.NewLayout = core1_0.ImageLayoutShaderReadOnlyOptimal
		barrier.SrcAccessMask = core1_0.AccessTransferRead
		barrier.DstAccessMask = core1_0.AccessShaderRead
		barrier.SrcQueueFamilyIndex = -1
		barrier.DstQueueFamilyIndex = -1
		err = app.deviceDriver.CmdPipelineBarrier(commandBuffer, core1_0.PipelineStageTransfer, core1_0.PipelineStageFragmentShader, 0, nil, nil, []core1_0.ImageMemoryBarrier{barrier})
		if err != nil {
			return err
		}

		mipWidth = nextMipWidth
		mipHeight = nextMipHeight
	}

	barrier.SubresourceRange.BaseMipLevel = mipLevels - 1
	barrier.OldLayout = core1_0.ImageLayoutTransferDstOptimal
	barrier.NewLayout = core1_0.ImageLayoutShaderReadOnlyOptimal
	barrier.SrcAccessMask = core1_0.AccessTransferWrite
	barrier.DstAccessMask = core1_0.AccessShaderRead

	err = app.deviceDriver.CmdPipelineBarrier(
		commandBuffer,
		core1_0.PipelineStageTransfer,
		core1_0.PipelineStageFragmentShader,
		0, nil, nil,
		[]core1_0.ImageMemoryBarrier{barrier})
	if err != nil {
		return err
	}

	return app.endSingleTimeCommands(commandBuffer)
}

func (app *HelloTriangleApplication) getMaxUsableSampleCount() (core1_0.SampleCountFlags, error) {
	properties, err := app.instanceDriver.GetPhysicalDeviceProperties(app.physicalDevice)
	if err != nil {
		return 0, err
	}

	counts := properties.Limits.FramebufferColorSampleCounts & properties.Limits.FramebufferDepthSampleCounts

	if (counts & core1_0.Samples64) != 0 {
		return core1_0.Samples64, nil
	}
	if (counts & core1_0.Samples32) != 0 {
		return core1_0.Samples32, nil
	}
	if (counts & core1_0.Samples16) != 0 {
		return core1_0.Samples16, nil
	}
	if (counts & core1_0.Samples8) != 0 {
		return core1_0.Samples8, nil
	}
	if (counts & core1_0.Samples4) != 0 {
		return core1_0.Samples4, nil
	}
	if (counts & core1_0.Samples2) != 0 {
		return core1_0.Samples2, nil
	}
	return core1_0.Samples1, nil
}

func (app *HelloTriangleApplication) createTextureImageView() error {
	var err error
	app.textureImageView, err = app.createImageView(app.textureImage, core1_0.FormatR8G8B8A8SRGB, core1_0.ImageAspectColor, app.mipLevels)
	return err
}

func (app *HelloTriangleApplication) createSampler() error {
	properties, err := app.instanceDriver.GetPhysicalDeviceProperties(app.physicalDevice)
	if err != nil {
		return err
	}

	app.textureSampler, _, err = app.deviceDriver.CreateSampler(nil, core1_0.SamplerCreateInfo{
		MagFilter:    core1_0.FilterLinear,
		MinFilter:    core1_0.FilterLinear,
		AddressModeU: core1_0.SamplerAddressModeRepeat,
		AddressModeV: core1_0.SamplerAddressModeRepeat,
		AddressModeW: core1_0.SamplerAddressModeRepeat,

		AnisotropyEnable: true,
		MaxAnisotropy:    properties.Limits.MaxSamplerAnisotropy,

		BorderColor: core1_0.BorderColorIntOpaqueBlack,

		MipmapMode: core1_0.SamplerMipmapModeLinear,
		MinLod:     0,
		MaxLod:     float32(app.mipLevels),
	})

	return err
}

func (app *HelloTriangleApplication) createImageView(image core1_0.Image, format core1_0.Format, aspect core1_0.ImageAspectFlags, mipLevels int) (core1_0.ImageView, error) {
	imageView, _, err := app.deviceDriver.CreateImageView(nil, core1_0.ImageViewCreateInfo{
		Image:    image,
		ViewType: core1_0.ImageViewType2D,
		Format:   format,
		SubresourceRange: core1_0.ImageSubresourceRange{
			AspectMask:     aspect,
			BaseMipLevel:   0,
			LevelCount:     mipLevels,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
	})
	return imageView, err
}

func (app *HelloTriangleApplication) createImage(width, height int, mipLevels int, numSamples core1_0.SampleCountFlags, format core1_0.Format, tiling core1_0.ImageTiling, usage core1_0.ImageUsageFlags, memoryProperties core1_0.MemoryPropertyFlags) (core1_0.Image, core1_0.DeviceMemory, error) {
	image, _, err := app.deviceDriver.CreateImage(nil, core1_0.ImageCreateInfo{
		ImageType: core1_0.ImageType2D,
		Extent: core1_0.Extent3D{
			Width:  width,
			Height: height,
			Depth:  1,
		},
		MipLevels:     mipLevels,
		ArrayLayers:   1,
		Format:        format,
		Tiling:        tiling,
		InitialLayout: core1_0.ImageLayoutUndefined,
		Usage:         usage,
		SharingMode:   core1_0.SharingModeExclusive,
		Samples:       numSamples,
	})
	if err != nil {
		return core1_0.Image{}, core1_0.DeviceMemory{}, err
	}

	memReqs := app.deviceDriver.GetImageMemoryRequirements(image)
	memoryIndex, err := app.findMemoryType(memReqs.MemoryTypeBits, memoryProperties)
	if err != nil {
		return core1_0.Image{}, core1_0.DeviceMemory{}, err
	}

	imageMemory, _, err := app.deviceDriver.AllocateMemory(nil, core1_0.MemoryAllocateInfo{
		AllocationSize:  memReqs.Size,
		MemoryTypeIndex: memoryIndex,
	})

	_, err = app.deviceDriver.BindImageMemory(image, imageMemory, 0)
	if err != nil {
		return core1_0.Image{}, core1_0.DeviceMemory{}, err
	}

	return image, imageMemory, nil
}

func (app *HelloTriangleApplication) transitionImageLayout(image core1_0.Image, format core1_0.Format, oldLayout core1_0.ImageLayout, newLayout core1_0.ImageLayout, mipLevels int) error {
	buffer, err := app.beginSingleTimeCommands()
	if err != nil {
		return err
	}

	var sourceStage, destStage core1_0.PipelineStageFlags
	var sourceAccess, destAccess core1_0.AccessFlags

	if oldLayout == core1_0.ImageLayoutUndefined && newLayout == core1_0.ImageLayoutTransferDstOptimal {
		sourceAccess = 0
		destAccess = core1_0.AccessTransferWrite
		sourceStage = core1_0.PipelineStageTopOfPipe
		destStage = core1_0.PipelineStageTransfer
	} else if oldLayout == core1_0.ImageLayoutTransferDstOptimal && newLayout == core1_0.ImageLayoutShaderReadOnlyOptimal {
		sourceAccess = core1_0.AccessTransferWrite
		destAccess = core1_0.AccessShaderRead
		sourceStage = core1_0.PipelineStageTransfer
		destStage = core1_0.PipelineStageFragmentShader
	} else {
		return errors.Errorf("unexpected layout transition: %s -> %s", oldLayout, newLayout)
	}

	err = app.deviceDriver.CmdPipelineBarrier(buffer, sourceStage, destStage, 0, nil, nil, []core1_0.ImageMemoryBarrier{
		{
			OldLayout:           oldLayout,
			NewLayout:           newLayout,
			SrcQueueFamilyIndex: -1,
			DstQueueFamilyIndex: -1,
			Image:               image,
			SubresourceRange: core1_0.ImageSubresourceRange{
				AspectMask:     core1_0.ImageAspectColor,
				BaseMipLevel:   0,
				LevelCount:     mipLevels,
				BaseArrayLayer: 0,
				LayerCount:     1,
			},
			SrcAccessMask: sourceAccess,
			DstAccessMask: destAccess,
		},
	})
	if err != nil {
		return err
	}

	return app.endSingleTimeCommands(buffer)
}

func (app *HelloTriangleApplication) copyBufferToImage(buffer core1_0.Buffer, image core1_0.Image, width, height int) error {
	cmdBuffer, err := app.beginSingleTimeCommands()
	if err != nil {
		return err
	}

	err = app.deviceDriver.CmdCopyBufferToImage(cmdBuffer, buffer, image, core1_0.ImageLayoutTransferDstOptimal,
		core1_0.BufferImageCopy{
			BufferOffset:      0,
			BufferRowLength:   0,
			BufferImageHeight: 0,

			ImageSubresource: core1_0.ImageSubresourceLayers{
				AspectMask:     core1_0.ImageAspectColor,
				MipLevel:       0,
				BaseArrayLayer: 0,
				LayerCount:     1,
			},
			ImageOffset: core1_0.Offset3D{X: 0, Y: 0, Z: 0},
			ImageExtent: core1_0.Extent3D{Width: width, Height: height, Depth: 1},
		})
	if err != nil {
		return err
	}

	return app.endSingleTimeCommands(cmdBuffer)
}

func writeData(driver core1_0.DeviceDriver, memory core1_0.DeviceMemory, offset int, data any) error {
	bufferSize := binary.Size(data)

	memoryPtr, _, err := driver.MapMemory(memory, offset, bufferSize, 0)
	if err != nil {
		return err
	}
	defer driver.UnmapMemory(memory)

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
		vert := Vertex{Position: vkngmath.Vec3[float32]{
			X: decoder.Vertices[vertInd*3],
			Y: decoder.Vertices[vertInd*3+1],
			Z: decoder.Vertices[vertInd*3+2],
		}, Color: vkngmath.Vec3[float32]{X: 1, Y: 1, Z: 1}}

		uvInd := face.Uvs[faceIndex]
		vert.TexCoord = vkngmath.Vec2[float32]{
			X: decoder.Uvs[uvInd*2],
			Y: 1.0 - decoder.Uvs[uvInd*2+1],
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

	stagingBuffer, stagingBufferMemory, err := app.createBuffer(bufferSize, core1_0.BufferUsageTransferSrc, core1_0.MemoryPropertyHostVisible|core1_0.MemoryPropertyHostCoherent)
	if stagingBuffer.Initialized() {
		defer app.deviceDriver.DestroyBuffer(stagingBuffer, nil)
	}
	if stagingBufferMemory.Initialized() {
		defer app.deviceDriver.FreeMemory(stagingBufferMemory, nil)
	}

	if err != nil {
		return err
	}

	err = writeData(app.deviceDriver, stagingBufferMemory, 0, app.vertices)
	if err != nil {
		return err
	}

	app.vertexBuffer, app.vertexBufferMemory, err = app.createBuffer(bufferSize, core1_0.BufferUsageTransferDst|core1_0.BufferUsageVertexBuffer, core1_0.MemoryPropertyDeviceLocal)
	if err != nil {
		return err
	}

	return app.copyBuffer(stagingBuffer, app.vertexBuffer, bufferSize)
}

func (app *HelloTriangleApplication) createIndexBuffer() error {
	bufferSize := binary.Size(app.indices)

	stagingBuffer, stagingBufferMemory, err := app.createBuffer(bufferSize, core1_0.BufferUsageTransferSrc, core1_0.MemoryPropertyHostVisible|core1_0.MemoryPropertyHostCoherent)
	if stagingBuffer.Initialized() {
		defer app.deviceDriver.DestroyBuffer(stagingBuffer, nil)
	}
	if stagingBufferMemory.Initialized() {
		defer app.deviceDriver.FreeMemory(stagingBufferMemory, nil)
	}

	if err != nil {
		return err
	}

	err = writeData(app.deviceDriver, stagingBufferMemory, 0, app.indices)
	if err != nil {
		return err
	}

	app.indexBuffer, app.indexBufferMemory, err = app.createBuffer(bufferSize, core1_0.BufferUsageTransferDst|core1_0.BufferUsageIndexBuffer, core1_0.MemoryPropertyDeviceLocal)
	if err != nil {
		return err
	}

	return app.copyBuffer(stagingBuffer, app.indexBuffer, bufferSize)
}

func (app *HelloTriangleApplication) createUniformBuffers() error {
	bufferSize := int(unsafe.Sizeof(UniformBufferObject{}))

	for i := 0; i < len(app.swapchainImages); i++ {
		buffer, memory, err := app.createBuffer(bufferSize, core1_0.BufferUsageUniformBuffer, core1_0.MemoryPropertyHostVisible|core1_0.MemoryPropertyHostCoherent)
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
	app.descriptorPool, _, err = app.deviceDriver.CreateDescriptorPool(nil, core1_0.DescriptorPoolCreateInfo{
		MaxSets: len(app.swapchainImages),
		PoolSizes: []core1_0.DescriptorPoolSize{
			{
				Type:            core1_0.DescriptorTypeUniformBuffer,
				DescriptorCount: len(app.swapchainImages),
			},
			{
				Type:            core1_0.DescriptorTypeCombinedImageSampler,
				DescriptorCount: len(app.swapchainImages),
			},
		},
	})
	return err
}

func (app *HelloTriangleApplication) createDescriptorSets() error {
	var allocLayouts []core1_0.DescriptorSetLayout
	for i := 0; i < len(app.swapchainImages); i++ {
		allocLayouts = append(allocLayouts, app.descriptorSetLayout)
	}

	var err error
	app.descriptorSets, _, err = app.deviceDriver.AllocateDescriptorSets(core1_0.DescriptorSetAllocateInfo{
		DescriptorPool: app.descriptorPool,
		SetLayouts:     allocLayouts,
	})
	if err != nil {
		return err
	}

	for i := 0; i < len(app.swapchainImages); i++ {
		err = app.deviceDriver.UpdateDescriptorSets([]core1_0.WriteDescriptorSet{
			{
				DstSet:          app.descriptorSets[i],
				DstBinding:      0,
				DstArrayElement: 0,

				DescriptorType: core1_0.DescriptorTypeUniformBuffer,

				BufferInfo: []core1_0.DescriptorBufferInfo{
					{
						Buffer: app.uniformBuffers[i],
						Offset: 0,
						Range:  int(unsafe.Sizeof(UniformBufferObject{})),
					},
				},
			},
			{
				DstSet:          app.descriptorSets[i],
				DstBinding:      1,
				DstArrayElement: 0,

				DescriptorType: core1_0.DescriptorTypeCombinedImageSampler,

				ImageInfo: []core1_0.DescriptorImageInfo{
					{
						ImageView:   app.textureImageView,
						Sampler:     app.textureSampler,
						ImageLayout: core1_0.ImageLayoutShaderReadOnlyOptimal,
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

func (app *HelloTriangleApplication) createBuffer(size int, usage core1_0.BufferUsageFlags, properties core1_0.MemoryPropertyFlags) (core1_0.Buffer, core1_0.DeviceMemory, error) {
	buffer, _, err := app.deviceDriver.CreateBuffer(nil, core1_0.BufferCreateInfo{
		Size:        size,
		Usage:       usage,
		SharingMode: core1_0.SharingModeExclusive,
	})
	if err != nil {
		return core1_0.Buffer{}, core1_0.DeviceMemory{}, err
	}

	memRequirements := app.deviceDriver.GetBufferMemoryRequirements(buffer)

	memoryTypeIndex, err := app.findMemoryType(memRequirements.MemoryTypeBits, properties)
	if err != nil {
		return buffer, core1_0.DeviceMemory{}, err
	}

	memory, _, err := app.deviceDriver.AllocateMemory(nil, core1_0.MemoryAllocateInfo{
		AllocationSize:  memRequirements.Size,
		MemoryTypeIndex: memoryTypeIndex,
	})
	if err != nil {
		return buffer, core1_0.DeviceMemory{}, err
	}

	_, err = app.deviceDriver.BindBufferMemory(buffer, memory, 0)
	return buffer, memory, err
}

func (app *HelloTriangleApplication) beginSingleTimeCommands() (core1_0.CommandBuffer, error) {
	buffers, _, err := app.deviceDriver.AllocateCommandBuffers(core1_0.CommandBufferAllocateInfo{
		CommandPool:        app.commandPool,
		Level:              core1_0.CommandBufferLevelPrimary,
		CommandBufferCount: 1,
	})
	if err != nil {
		return core1_0.CommandBuffer{}, err
	}

	buffer := buffers[0]
	_, err = app.deviceDriver.BeginCommandBuffer(buffer, core1_0.CommandBufferBeginInfo{
		Flags: core1_0.CommandBufferUsageOneTimeSubmit,
	})
	return buffer, err
}

func (app *HelloTriangleApplication) endSingleTimeCommands(buffer core1_0.CommandBuffer) error {
	_, err := app.deviceDriver.EndCommandBuffer(buffer)
	if err != nil {
		return err
	}

	_, err = app.deviceDriver.QueueSubmit(app.graphicsQueue, nil,
		core1_0.SubmitInfo{
			CommandBuffers: []core1_0.CommandBuffer{buffer},
		})

	if err != nil {
		return err
	}

	_, err = app.deviceDriver.QueueWaitIdle(app.graphicsQueue)
	if err != nil {
		return err
	}

	app.deviceDriver.FreeCommandBuffers(buffer)
	return nil
}

func (app *HelloTriangleApplication) copyBuffer(srcBuffer core1_0.Buffer, dstBuffer core1_0.Buffer, size int) error {
	buffer, err := app.beginSingleTimeCommands()
	if err != nil {
		return err
	}

	err = app.deviceDriver.CmdCopyBuffer(buffer, srcBuffer, dstBuffer,
		core1_0.BufferCopy{
			SrcOffset: 0,
			DstOffset: 0,
			Size:      size,
		})
	if err != nil {
		return err
	}

	return app.endSingleTimeCommands(buffer)
}

func (app *HelloTriangleApplication) findMemoryType(typeFilter uint32, properties core1_0.MemoryPropertyFlags) (int, error) {
	memProperties := app.instanceDriver.GetPhysicalDeviceMemoryProperties(app.physicalDevice)
	for i, memoryType := range memProperties.MemoryTypes {
		typeBit := uint32(1 << i)

		if (typeFilter&typeBit) != 0 && (memoryType.PropertyFlags&properties) == properties {
			return i, nil
		}
	}

	return 0, errors.New("failed to find any suitable memory type!")
}

func (app *HelloTriangleApplication) createCommandBuffers() error {

	buffers, _, err := app.deviceDriver.AllocateCommandBuffers(core1_0.CommandBufferAllocateInfo{
		CommandPool:        app.commandPool,
		Level:              core1_0.CommandBufferLevelPrimary,
		CommandBufferCount: len(app.swapchainImages),
	})
	if err != nil {
		return err
	}
	app.commandBuffers = buffers

	for bufferIdx, buffer := range buffers {
		_, err = app.deviceDriver.BeginCommandBuffer(buffer, core1_0.CommandBufferBeginInfo{})
		if err != nil {
			return err
		}

		err = app.deviceDriver.CmdBeginRenderPass(buffer, core1_0.SubpassContentsInline,
			core1_0.RenderPassBeginInfo{
				RenderPass:  app.renderPass,
				Framebuffer: app.swapchainFramebuffers[bufferIdx],
				RenderArea: core1_0.Rect2D{
					Offset: core1_0.Offset2D{X: 0, Y: 0},
					Extent: app.swapchainExtent,
				},
				ClearValues: []core1_0.ClearValue{
					core1_0.ClearValueFloat{0, 0, 0, 1},
					core1_0.ClearValueDepthStencil{Depth: 1.0, Stencil: 0},
				},
			})
		if err != nil {
			return err
		}

		app.deviceDriver.CmdBindPipeline(buffer, core1_0.PipelineBindPointGraphics, app.graphicsPipeline)
		app.deviceDriver.CmdBindVertexBuffers(buffer, 0, []core1_0.Buffer{app.vertexBuffer}, []int{0})
		app.deviceDriver.CmdBindIndexBuffer(buffer, app.indexBuffer, 0, core1_0.IndexTypeUInt32)
		app.deviceDriver.CmdBindDescriptorSets(buffer, core1_0.PipelineBindPointGraphics, app.pipelineLayout, 0, []core1_0.DescriptorSet{
			app.descriptorSets[bufferIdx],
		}, nil)
		app.deviceDriver.CmdDrawIndexed(buffer, len(app.indices), 1, 0, 0, 0)
		app.deviceDriver.CmdEndRenderPass(buffer)

		_, err = app.deviceDriver.EndCommandBuffer(buffer)
		if err != nil {
			return err
		}
	}

	return nil
}

func (app *HelloTriangleApplication) createSyncObjects() error {
	for i := 0; i < len(app.swapchainImages); i++ {
		semaphore, _, err := app.deviceDriver.CreateSemaphore(nil, core1_0.SemaphoreCreateInfo{})
		if err != nil {
			return err
		}

		app.imageAvailableSemaphore = append(app.imageAvailableSemaphore, semaphore)

		fence, _, err := app.deviceDriver.CreateFence(nil, core1_0.FenceCreateInfo{
			Flags: core1_0.FenceCreateSignaled,
		})
		if err != nil {
			return err
		}

		app.inFlightFence = append(app.inFlightFence, fence)
	}

	for i := 0; i < len(app.swapchainImages); i++ {
		semaphore, _, err := app.deviceDriver.CreateSemaphore(nil, core1_0.SemaphoreCreateInfo{})
		if err != nil {
			return err
		}

		app.renderFinishedSemaphore = append(app.renderFinishedSemaphore, semaphore)

		app.imagesInFlight = append(app.imagesInFlight, core1_0.Fence{})
	}

	return nil
}

func (app *HelloTriangleApplication) drawFrame() error {
	_, err := app.deviceDriver.WaitForFences(true, common.NoTimeout, app.inFlightFence[app.currentFrame])
	if err != nil {
		return err
	}

	imageIndex, res, err := app.swapchainExtension.AcquireNextImage(app.swapchain, common.NoTimeout, &app.imageAvailableSemaphore[app.currentFrame], nil)
	if res == khr_swapchain.VKErrorOutOfDate {
		return app.recreateSwapChain()
	} else if err != nil {
		return err
	}

	if app.imagesInFlight[imageIndex].Initialized() {
		_, err := app.deviceDriver.WaitForFences(true, common.NoTimeout, app.imagesInFlight[imageIndex])
		if err != nil {
			return err
		}
	}
	app.imagesInFlight[imageIndex] = app.inFlightFence[app.currentFrame]

	_, err = app.deviceDriver.ResetFences(app.inFlightFence[app.currentFrame])
	if err != nil {
		return err
	}

	err = app.updateUniformBuffer(imageIndex)
	if err != nil {
		return err
	}

	_, err = app.deviceDriver.QueueSubmit(app.graphicsQueue, &app.inFlightFence[app.currentFrame],
		core1_0.SubmitInfo{
			WaitSemaphores:   []core1_0.Semaphore{app.imageAvailableSemaphore[app.currentFrame]},
			WaitDstStageMask: []core1_0.PipelineStageFlags{core1_0.PipelineStageColorAttachmentOutput},
			CommandBuffers:   []core1_0.CommandBuffer{app.commandBuffers[imageIndex]},
			SignalSemaphores: []core1_0.Semaphore{app.renderFinishedSemaphore[imageIndex]},
		})
	if err != nil {
		return err
	}

	res, err = app.swapchainExtension.QueuePresent(app.presentQueue, khr_swapchain.PresentInfo{
		WaitSemaphores: []core1_0.Semaphore{app.renderFinishedSemaphore[imageIndex]},
		Swapchains:     []khr_swapchain.Swapchain{app.swapchain},
		ImageIndices:   []int{imageIndex},
	})

	if res == khr_swapchain.VKErrorOutOfDate || res == khr_swapchain.VKSuboptimal {
		return app.recreateSwapChain()
	} else if err != nil {
		return err
	}
	app.currentFrame = (app.currentFrame + 1) % MaxFramesInFlight

	return nil
}

func (app *HelloTriangleApplication) updateUniformBuffer(currentImage int) error {
	currentTime := hrtime.Now().Seconds()
	timePeriod := math.Mod(currentTime, 4.0)

	ubo := UniformBufferObject{}
	ubo.Model.SetRotationZ(timePeriod * math.Pi / 2.0)
	ubo.View.SetLookAt(
		&vkngmath.Vec3[float32]{X: 2, Y: 2, Z: 2},
		&vkngmath.Vec3[float32]{X: 0, Y: 0, Z: 0},
		&vkngmath.Vec3[float32]{X: 0, Y: 0, Z: 1},
	)
	aspectRatio := float32(app.swapchainExtent.Width) / float32(app.swapchainExtent.Height)

	near := float32(0.1)
	far := float32(10.0)
	fovy := math.Pi / 4.0

	ubo.Proj.SetPerspective(fovy, aspectRatio, near, far)

	err := writeData(app.deviceDriver, app.uniformBuffersMemory[currentImage], 0, &ubo)
	return err
}

func (app *HelloTriangleApplication) chooseSwapSurfaceFormat(availableFormats []khr_surface.SurfaceFormat) khr_surface.SurfaceFormat {
	for _, format := range availableFormats {
		if format.Format == core1_0.FormatB8G8R8A8SRGB && format.ColorSpace == khr_surface.ColorSpaceSRGBNonlinear {
			return format
		}
	}

	return availableFormats[0]
}

func (app *HelloTriangleApplication) chooseSwapPresentMode(availablePresentModes []khr_surface.PresentMode) khr_surface.PresentMode {
	for _, presentMode := range availablePresentModes {
		if presentMode == khr_surface.PresentModeMailbox {
			return presentMode
		}
	}

	return khr_surface.PresentModeFIFO
}

func (app *HelloTriangleApplication) chooseSwapExtent(capabilities *khr_surface.SurfaceCapabilities) core1_0.Extent2D {
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

	return core1_0.Extent2D{Width: width, Height: height}
}

func (app *HelloTriangleApplication) querySwapChainSupport(device core1_0.PhysicalDevice) (SwapChainSupportDetails, error) {
	var details SwapChainSupportDetails
	var err error

	details.Capabilities, _, err = app.surfaceExtension.GetPhysicalDeviceSurfaceCapabilities(app.surface, device)
	if err != nil {
		return details, err
	}

	details.Formats, _, err = app.surfaceExtension.GetPhysicalDeviceSurfaceFormats(app.surface, device)
	if err != nil {
		return details, err
	}

	details.PresentModes, _, err = app.surfaceExtension.GetPhysicalDeviceSurfacePresentModes(app.surface, device)
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

	features := app.instanceDriver.GetPhysicalDeviceFeatures(device)
	return indices.IsComplete() && extensionsSupported && swapChainAdequate && features.SamplerAnisotropy
}

func (app *HelloTriangleApplication) checkDeviceExtensionSupport(device core1_0.PhysicalDevice) bool {
	extensions, _, err := app.instanceDriver.EnumerateDeviceExtensionProperties(device)
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
	queueFamilies := app.instanceDriver.GetPhysicalDeviceQueueFamilyProperties(device)

	for queueFamilyIdx, queueFamily := range queueFamilies {
		if (queueFamily.QueueFlags & core1_0.QueueGraphics) != 0 {
			indices.GraphicsFamily = new(int)
			*indices.GraphicsFamily = queueFamilyIdx
		}

		supported, _, err := app.surfaceExtension.GetPhysicalDeviceSurfaceSupport(app.surface, device, queueFamilyIdx)
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

func (app *HelloTriangleApplication) logDebug(msgType ext_debug_utils.DebugUtilsMessageTypeFlags, severity ext_debug_utils.DebugUtilsMessageSeverityFlags, data *ext_debug_utils.DebugUtilsMessengerCallbackData) bool {
	log.Printf("[%s %s] - %s", severity, msgType, data.Message)
	return false
}

func main() {
	runtime.LockOSThread()
	app := &HelloTriangleApplication{
		msaaSamples: core1_0.Samples1,
	}

	err := app.Run()
	if err != nil {
		log.Fatalf("%+v\n", err)
	}
}

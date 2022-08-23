package main

import (
	"github.com/cockroachdb/errors"
	"github.com/veandco/go-sdl2/sdl"
	"github.com/vkngwrapper/core/v2"
	"github.com/vkngwrapper/core/v2/common"
	"github.com/vkngwrapper/core/v2/core1_0"
	"github.com/vkngwrapper/extensions/v2/ext_debug_utils"
	"github.com/vkngwrapper/extensions/v2/khr_portability_enumeration"
	"github.com/vkngwrapper/extensions/v2/khr_portability_subset"
	"github.com/vkngwrapper/extensions/v2/khr_surface"
	"github.com/vkngwrapper/extensions/v2/khr_swapchain"
	vkng_sdl2 "github.com/vkngwrapper/integrations/sdl2/v2"
	"log"
)

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

type HelloTriangleApplication struct {
	window *sdl.Window
	loader core.Loader

	instance       core1_0.Instance
	debugMessenger ext_debug_utils.DebugUtilsMessenger
	surface        khr_surface.Surface

	physicalDevice core1_0.PhysicalDevice
	device         core1_0.Device

	graphicsQueue core1_0.Queue
	presentQueue  core1_0.Queue

	swapchainExtension   khr_swapchain.Extension
	swapchain            khr_swapchain.Swapchain
	swapchainImages      []core1_0.Image
	swapchainImageFormat core1_0.Format
	swapchainExtent      core1_0.Extent2D
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

	return app.createSwapchain()
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
	instanceOptions := core1_0.InstanceCreateInfo{
		ApplicationName:    "Hello Triangle",
		ApplicationVersion: common.CreateVersion(1, 0, 0),
		EngineName:         "No Engine",
		EngineVersion:      common.CreateVersion(1, 0, 0),
		APIVersion:         common.Vulkan1_2,
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
			instanceOptions.EnabledLayerNames = append(instanceOptions.EnabledLayerNames, layer)
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
	debugLoader := ext_debug_utils.CreateExtensionFromInstance(app.instance)
	app.debugMessenger, _, err = debugLoader.CreateDebugUtilsMessenger(app.instance, nil, app.debugMessengerOptions())
	if err != nil {
		return err
	}

	return nil
}

func (app *HelloTriangleApplication) createSurface() error {
	surfaceLoader := khr_surface.CreateExtensionFromInstance(app.instance)

	surface, err := vkng_sdl2.CreateSurface(app.instance, surfaceLoader, app.window)
	if err != nil {
		return err
	}

	app.surface = surface
	return nil
}

func (app *HelloTriangleApplication) pickPhysicalDevice() error {
	physicalDevices, _, err := app.instance.EnumeratePhysicalDevices()
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
	extensions, _, err := app.physicalDevice.EnumerateDeviceExtensionProperties()
	if err != nil {
		return err
	}

	_, supported := extensions[khr_portability_subset.ExtensionName]
	if supported {
		extensionNames = append(extensionNames, khr_portability_subset.ExtensionName)
	}

	app.device, _, err = app.physicalDevice.CreateDevice(nil, core1_0.DeviceCreateInfo{
		QueueCreateInfos:      queueFamilyOptions,
		EnabledFeatures:       &core1_0.PhysicalDeviceFeatures{},
		EnabledExtensionNames: extensionNames,
	})
	if err != nil {
		return err
	}

	app.graphicsQueue = app.device.GetQueue(*indices.GraphicsFamily, 0)
	app.presentQueue = app.device.GetQueue(*indices.PresentFamily, 0)
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

	swapchain, _, err := app.swapchainExtension.CreateSwapchain(app.device, nil, khr_swapchain.SwapchainCreateInfo{
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

	details.Capabilities, _, err = app.surface.PhysicalDeviceSurfaceCapabilities(device)
	if err != nil {
		return details, err
	}

	details.Formats, _, err = app.surface.PhysicalDeviceSurfaceFormats(device)
	if err != nil {
		return details, err
	}

	details.PresentModes, _, err = app.surface.PhysicalDeviceSurfacePresentModes(device)
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
	extensions, _, err := device.EnumerateDeviceExtensionProperties()
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
		if (queueFamily.QueueFlags & core1_0.QueueGraphics) != 0 {
			indices.GraphicsFamily = new(int)
			*indices.GraphicsFamily = queueFamilyIdx
		}

		supported, _, err := app.surface.PhysicalDeviceSurfaceSupport(device, queueFamilyIdx)
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
	app := &HelloTriangleApplication{}

	err := app.Run()
	if err != nil {
		log.Fatalf("%+v\n", err)
	}
}

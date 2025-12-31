package main

import (
	"log"

	"github.com/pkg/errors"
	"github.com/veandco/go-sdl2/sdl"
	"github.com/vkngwrapper/core/v3"
	"github.com/vkngwrapper/core/v3/common"
	"github.com/vkngwrapper/core/v3/core1_0"
	"github.com/vkngwrapper/extensions/v3/ext_debug_utils"
	"github.com/vkngwrapper/extensions/v3/khr_portability_enumeration"
	"github.com/vkngwrapper/extensions/v3/khr_portability_subset"
	"github.com/vkngwrapper/extensions/v3/khr_surface"
	vkng_sdl2 "github.com/vkngwrapper/integrations/sdl2/v3"
)

var validationLayers = []string{"VK_LAYER_KHRONOS_validation"}

const enableValidationLayers = true

type QueueFamilyIndices struct {
	GraphicsFamily *int
	PresentFamily  *int
}

func (i *QueueFamilyIndices) IsComplete() bool {
	return i.GraphicsFamily != nil && i.PresentFamily != nil
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

	return app.createLogicalDevice()
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
		_ = app.window.Destroy()
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
		QueueCreateInfos:      queueFamilyOptions,
		EnabledFeatures:       &core1_0.PhysicalDeviceFeatures{},
		EnabledExtensionNames: extensionNames,
	})
	if err != nil {
		return err
	}

	app.graphicsQueue = app.deviceDriver.GetQueue(*indices.GraphicsFamily, 0)
	return nil
}

func (app *HelloTriangleApplication) isDeviceSuitable(device core1_0.PhysicalDevice) bool {
	indices, err := app.findQueueFamilies(device)
	if err != nil {
		return false
	}

	return indices.IsComplete()
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
	app := &HelloTriangleApplication{}

	err := app.Run()
	if err != nil {
		log.Fatalf("%+v\n", err)
	}
}

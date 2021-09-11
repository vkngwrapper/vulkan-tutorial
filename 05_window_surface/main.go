package main

import (
	"github.com/CannibalVox/VKng/core"
	"github.com/CannibalVox/VKng/core/loader"
	"github.com/CannibalVox/VKng/core/resources"
	ext_debugutils "github.com/CannibalVox/VKng/extensions/debugutils"
	"github.com/CannibalVox/VKng/extensions/surface"
	ext_surface_sdl2 "github.com/CannibalVox/VKng/extensions/surface_sdl"
	"github.com/palantir/stacktrace"
	"github.com/veandco/go-sdl2/sdl"
	"log"
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
	window *sdl.Window
	loader loader.Loader

	instance       resources.Instance
	debugMessenger ext_debugutils.Messenger
	surface        ext_surface.Surface

	physicalDevice resources.PhysicalDevice
	device         resources.Device

	graphicsQueue resources.Queue
	presentQueue  resources.Queue
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

	app.loader, err = loader.CreateLoaderFromProcAddr(sdl.VulkanGetVkGetInstanceProcAddr())
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

func (app *HelloTriangleApplication) createInstance() error {
	instanceOptions := &resources.InstanceOptions{
		ApplicationName:    "Hello Triangle",
		ApplicationVersion: core.CreateVersion(1, 0, 0),
		EngineName:         "No Engine",
		EngineVersion:      core.CreateVersion(1, 0, 0),
		VulkanVersion:      core.Vulkan1_2,
	}

	// Add extensions
	sdlExtensions := app.window.VulkanGetInstanceExtensions()
	extensions, _, err := resources.AvailableExtensions(app.loader)
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
	layers, _, err := resources.AvailableLayers(app.loader)
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

	app.instance, _, err = resources.CreateInstance(app.loader, instanceOptions)
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
	app.debugMessenger, _, err = ext_debugutils.CreateMessenger(app.instance, app.debugMessengerOptions())
	if err != nil {
		return err
	}

	return nil
}

func (app *HelloTriangleApplication) createSurface() error {
	surface, _, err := ext_surface_sdl2.CreateSurface(app.instance, app.window)
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

	var queueFamilyOptions []*resources.QueueFamilyOptions
	queuePriority := float32(1.0)
	for _, queueFamily := range uniqueQueueFamilies {
		queueFamilyOptions = append(queueFamilyOptions, &resources.QueueFamilyOptions{
			QueueFamilyIndex: queueFamily,
			QueuePriorities:  []float32{queuePriority},
		})
	}

	var extensionNames []string

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

	app.device, _, err = app.physicalDevice.CreateDevice(&resources.DeviceOptions{
		QueueFamilies:   queueFamilyOptions,
		EnabledFeatures: &core.PhysicalDeviceFeatures{},
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

func (app *HelloTriangleApplication) isDeviceSuitable(device resources.PhysicalDevice) bool {
	indices, err := app.findQueueFamilies(device)
	if err != nil {
		return false
	}

	return indices.IsComplete()
}

func (app *HelloTriangleApplication) findQueueFamilies(device resources.PhysicalDevice) (QueueFamilyIndices, error) {
	indices := QueueFamilyIndices{}
	queueFamilies, err := device.QueueFamilyProperties()
	if err != nil {
		return indices, err
	}

	for queueFamilyIdx, queueFamily := range queueFamilies {
		if (queueFamily.Flags & core.Graphics) != 0 {
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
	app := &HelloTriangleApplication{}

	err := app.Run()
	if err != nil {
		log.Fatalln(err)
	}
}

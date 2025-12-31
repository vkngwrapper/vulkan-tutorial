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
)

var validationLayers = []string{"VK_LAYER_KHRONOS_validation"}

const enableValidationLayers = true

type HelloTriangleApplication struct {
	globalDriver   core1_0.GlobalDriver
	instanceDriver core1_0.CoreInstanceDriver
	debugExtension ext_debug_utils.ExtensionDriver
	window         *sdl.Window

	debugMessenger ext_debug_utils.DebugUtilsMessenger
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

	app.globalDriver, err = core.CreateSystemDriver()
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

	return app.setupDebugMessenger()
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
	if app.debugMessenger.Initialized() {
		app.debugExtension.DestroyDebugUtilsMessenger(app.debugMessenger, nil)
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

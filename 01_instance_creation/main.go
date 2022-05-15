package main

import (
	"github.com/CannibalVox/VKng/core"
	"github.com/CannibalVox/VKng/core/common"
	"github.com/CannibalVox/VKng/core/core1_0"
	"github.com/cockroachdb/errors"
	"github.com/veandco/go-sdl2/sdl"
	"log"
)

type HelloTriangleApplication struct {
	loader core.Loader
	window *sdl.Window

	instance core1_0.Instance
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

	app.loader, err = core.CreateStaticLinkedLoader()
	if err != nil {
		return err
	}

	return nil
}

func (app *HelloTriangleApplication) initVulkan() error {
	return app.createInstance()
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
	if app.instance != nil {
		app.instance.Destroy(nil)
	}

	if app.window != nil {
		app.window.Destroy()
	}
	sdl.Quit()
}

func (app *HelloTriangleApplication) createInstance() error {
	instanceOptions := core1_0.InstanceCreateOptions{
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

	app.instance, _, err = app.loader.CreateInstance(nil, instanceOptions)
	if err != nil {
		return err
	}

	return nil
}

func main() {
	app := &HelloTriangleApplication{}

	err := app.Run()
	if err != nil {
		log.Fatalf("%+v\n", err)
	}
}

# Vulkan-Tutorial.com

This is a go port of the example code at vulkan-tutorial.com, 
 using vkngwrapper as a wrapper library.  Each subfolder pertains to a
 single step of the tutorial, and I have tried to strike a balance
 between making the tutorial code match the C++ code as closely
 as possible while still being vaguely idiomatic.  You should
 be able to use the code files here as a reference while working
 through the vulkan tutorial.

To learn more about vkngwrapper, check out [the core repository](https://github.com/vkngwrapper/core)!

---
* [Rights](#rights)
* [Executing This Code](#executing-this-code)
* [Notable Changes From C++](#notable-changes-from-c)
  * [SDL2 Instead of GLFW](#sdl2-instead-of-glfw)
  * [No Device Layers](#no-device-layers)
  * [Vulkan Portability Subset](#vulkan-portability-subset)
  * [Go Embed](#go-embed)
* [Drawing a Triangle](#drawing-a-triangle)
  * [Setup](#setup)
    * [Base Code](#base-code)
    * [Instance](#instance)
    * [Validation layers](#validation-layers)
    * [Physical devices and queue families](#physical-devices-and-queue-families)
    * [Logical devices and queues](#logical-device-and-queues)
  * [Presentation](#presentation)
    * [Window surface](#window-surface)
    * [Swapchain](#swapchain)
    * [Image views](#image-views)
  * [Graphics pipeline basics](#graphics-pipeline-basics)
    * [Introduction](#introduction)
    * [Shader Modules](#shader-modules)
    * [Fixed functions](#fixed-functions)
    * [Render passes](#render-passes)
    * [Conclusion](#conclusion)
  * [Drawing](#drawing)
    * [Framebuffers](#framebuffers)
    * [Command buffers](#command-buffers)
    * [Rendering and presentation](#rendering-and-presentation)
* [Swapchain Recreation](#swapchain-recreation)
* [Vertex buffers](#vertex-buffers)
  * [Vertex input description](#vertex-input-description)
  * [Vertex buffer creation](#vertex-buffer-creation)
  * [Staging buffer](#staging-buffer)
  * [Index buffer](#index-buffer)
* [Uniform buffers](#uniform-buffers)
  * [Descriptor layout and buffer](#descriptor-layout-and-buffer)
  * [Descriptor pool and sets](#descriptor-pool-and-sets)
* [Texture mapping](#texture-mapping)
  * [Images](#images)
  * [Image view and sampler](#image-view-and-sampler)
  * [Combined image sampler](#combined-image-sampler)
* [Depth Buffering](#depth-buffering)
* [Loading Models](#loading-models)
* [Generating Mipmaps](#generating-mipmaps)
* [Multisampling](#multisampling)

## Rights

The vulkan tutorial's source and licensing information can be
 found at https://github.com/Overv/VulkanTutorial and are licensed
 under the CC BY-SA 4.0 license or the CC0 1.0 Universal license.  The
 example code in this folder and its subfolders is also licensed under the CC0 1.0 Universal
 license found [here](https://creativecommons.org/publicdomain/zero/1.0/).
 Code outside this directory may be licensed differently.

Images and meshes in this directory were obtained from the vulkan
 tutorial and are licensed under the CC BY-SA 4.0 license.

## Executing This Code

Before this code can be executed, you will need to install [the Vulkan SDK](https://www.lunarg.com/vulkan-sdk/)
 for your operating system.Additionally, it may be necessary to download SDL2 using your local package
 manager. For more information, see [go-sdl2 requirements](https://github.com/veandco/go-sdl2#requirements).

## Notable Changes From C++

In order to best support this code as an idiomatic Golang example, there are a few differences between
 this code and the default C++ code provided at http://vulkan-tutorial.com/ - they are listed here and
 reasoning is provided.

### SDL2 Instead of GLFW

This example code uses [go-sdl2](https://github.com/veandco/go-sdl2) as its windowing system, with Surface
 support provided via [integrations/sdl2](https://github.com/vkngwrapper/integrations/sdl2). The primary
 reason for this is that go-sdl2's level of support is far, far better than any GLFW wrapper for Go.

### No Device layers

[Step 2](#validation-layers) of the tutorial instructs users to apply a validation layer to both the Vulkan
 Instance and the Device. However, Device layers were deprecated before Vulkan 1.0 was released, and is 
 not necessary when activating validation behavior. As a result, vkngwrapper does not support Device layers
 and we do not apply them in this tutorial.

### Vulkan Portability Subset

Beginning with [Step 4](#logical-device-and-queues), we activate the `VK_KHR_portability_subset` extension
 in the logical Device on creation, when it is available. Doing so allows this tutorial to run on hardware
 that does not support the full Vulkan spec, such as Mac laptops.

### Go Embed

Asset files are loaded from disk using [//go:embed](https://pkg.go.dev/embed). This makes it very easy
 to package each step's assets with the step itself and load the assets from disk with a minimum of 
 confusion.

## Drawing a Triangle
### Setup
#### Base Code

[Read the tutorial](https://vulkan-tutorial.com/Drawing_a_triangle/Setup/Base_code)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/00_base_code.cpp)

[Go code](steps/00_base_code/main.go)

#### Instance

[Read the tutorial](https://vulkan-tutorial.com/Drawing_a_triangle/Setup/Instance)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/01_instance_creation.cpp)

[Go code](steps/01_instance_creation/main.go)

[Diff](diffs/01_instance_creation.diff)

#### Validation layers

[Read the tutorial](https://vulkan-tutorial.com/Drawing_a_triangle/Setup/Validation_layers)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/02_validation_layers.cpp)

[Go code](steps/02_validation_layers/main.go)

[Diff](diffs/02_validation_layers.diff)

#### Physical devices and queue families

[Read the tutorial](https://vulkan-tutorial.com/Drawing_a_triangle/Setup/Physical_devices_and_queue_families)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/03_physical_device_selection.cpp)

[Go code](steps/03_physical_device_selection/main.go)

[Diff](diffs/03_physical_device_selection.diff)


#### Logical device and queues

[Read the tutorial](https://vulkan-tutorial.com/Drawing_a_triangle/Setup/Logical_device_and_queues)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/04_logical_device.cpp)

[Go code](steps/04_logical_device/main.go)

[Diff](diffs/04_logical_device.diff)

### Presentation

#### Window surface

[Read the tutorial](https://vulkan-tutorial.com/Drawing_a_triangle/Presentation/Window_surface)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/05_window_surface.cpp)

[Go code](steps/05_window_surface/main.go)

[Diffs](diffs/05_window_surface.diff)

#### Swapchain

[Read the tutorial](https://vulkan-tutorial.com/Drawing_a_triangle/Presentation/Swap_chain)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/06_swap_chain_creation.cpp)

[Go code](steps/06_swapchain/main.go)

[Diffs](diffs/06_swapchain.diff)

#### Image views

[Read the tutorial](https://vulkan-tutorial.com/Drawing_a_triangle/Presentation/Image_views)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/07_image_views.cpp)

[Go code](steps/07_image_views/main.go)

[Diffs](diffs/07_image_views.diff)

### Graphics pipeline basics
#### Introduction

[Read the tutorial](https://vulkan-tutorial.com/Drawing_a_triangle/Graphics_pipeline_basics)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/08_graphics_pipeline.cpp)

[Go code](steps/08_graphics_pipeline/main.go)

[Diffs](diffs/08_graphics_pipeline.diff)

#### Shader Modules

[Read the tutorial](https://vulkan-tutorial.com/Drawing_a_triangle/Graphics_pipeline_basics/Shader_modules)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/09_shader_modules.cpp)

[Go code](steps/09_shader_modules/main.go)

[Diffs](diffs/09_shader_modules.diff)

#### Fixed functions

[Read the tutorial](https://vulkan-tutorial.com/Drawing_a_triangle/Graphics_pipeline_basics/Fixed_functions)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/10_fixed_functions.cpp)

[Go code](steps/10_fixed_functions/main.go)

[Diffs](diffs/10_fixed_functions.diff)

#### Render passes

[Read the tutorial](https://vulkan-tutorial.com/Drawing_a_triangle/Graphics_pipeline_basics/Render_passes)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/11_render_passes.cpp)

[Go code](steps/11_render_passes/main.go)

[Diffs](diffs/11_render_passes.diff)

#### Conclusion 

[Read the tutorial](https://vulkan-tutorial.com/Drawing_a_triangle/Graphics_pipeline_basics/Conclusion)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/12_graphics_pipeline_complete.cpp)

[Go code](steps/12_graphics_pipeline_complete/main.go)

[Diffs](diffs/12_graphics_pipeline_complete.diff)

### Drawing
#### Framebuffers

[Read the tutorial](https://vulkan-tutorial.com/Drawing_a_triangle/Drawing/Framebuffers)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/13_framebuffers.cpp)

[Go code](steps/13_framebuffers/main.go)

[Diffs](diffs/13_framebuffers.diff)

#### Command buffers

[Read the tutorial](https://vulkan-tutorial.com/Drawing_a_triangle/Drawing/Command_buffers)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/14_command_buffers.cpp)

[Go code](steps/14_command_buffers/main.go)

[Diffs](diffs/14_command_buffers.diff)

#### Rendering and presentation

[Read the tutorial](https://vulkan-tutorial.com/Drawing_a_triangle/Drawing/Rendering_and_presentation)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/15_hello_triangle.cpp)

[Go code](steps/15_hello_triangle/main.go)

[Diffs](diffs/15_hello_triangle.diff)

## Swapchain Recreation

[Read the tutorial](https://vulkan-tutorial.com/Drawing_a_triangle/Swap_chain_recreation)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/16_swap_chain_recreation.cpp)

[Go code](steps/16_swap_chain_recreation/main.go)

[Diffs](diffs/16_swap_chain_recreation.diff)

## Vertex buffers
### Vertex input description

*(Will cause Validation Layer errors, but that will be fixed in the next chapter)*

[Read the tutorial](https://vulkan-tutorial.com/Vertex_buffers/Vertex_input_description)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/17_vertex_input.cpp)

[Go code](steps/17_vertex_input/main.go)

[Diffs](diffs/17_vertex_input.diff)

### Vertex buffer creation

[Read the tutorial](https://vulkan-tutorial.com/Vertex_buffers/Vertex_buffer_creation)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/18_vertex_buffer.cpp)

[Go code](steps/18_vertex_buffer/main.go)

[Diffs](diffs/18_vertex_buffer.diff)

### Staging buffer

[Read the tutorial](https://vulkan-tutorial.com/Vertex_buffers/Staging_buffer)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/19_staging_buffer.cpp)

[Go code](steps/19_staging_buffer/main.go)

[Diffs](diffs/19_staging_buffer.diff)

### Index buffer

[Read the tutorial](https://vulkan-tutorial.com/Vertex_buffers/Index_buffer)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/20_index_buffer.cpp)

[Go code](steps/20_index_buffer/main.go)

[Diffs](diffs/20_index_buffer.diff)

## Uniform buffers
### Descriptor layout and buffer

[Read the tutorial](https://vulkan-tutorial.com/Uniform_buffers/Descriptor_layout_and_buffer)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/21_descriptor_layout.cpp)

[Go code](steps/21_descriptor_layout/main.go)

[Diffs](diffs/21_descriptor_layout.diff)

### Descriptor pool and sets

[Read the tutorial](https://vulkan-tutorial.com/Uniform_buffers/Descriptor_pool_and_sets)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/22_descriptor_sets.cpp)

[Go code](steps/22_descriptor_sets/main.go)

[Diffs](diffs/22_descriptor_sets.diff)

## Texture mapping
### Images

[Read the tutorial](https://vulkan-tutorial.com/Texture_mapping/Images)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/23_texture_image.cpp)

[Go code](steps/23_texture_image/main.go)

[Diffs](diffs/23_texture_image.diff)

### Image view and sampler

[Read the tutorial](https://vulkan-tutorial.com/Texture_mapping/Image_view_and_sampler)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/24_sampler.cpp)

[Go code](steps/24_sampler/main.go)

[Diffs](diffs/24_sampler.diff)

### Combined image sampler

[Read the tutorial](https://vulkan-tutorial.com/Texture_mapping/Combined_image_sampler)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/25_texture_mapping.cpp)

[Go code](steps/25_texture_mapping/main.go)

[Diffs](diffs/25_texture_mapping.diff)

## Depth buffering

[Read the tutorial](https://vulkan-tutorial.com/Depth_buffering)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/26_depth_buffering.cpp)

[Go code](steps/26_depth_buffering/main.go)

[Diffs](diffs/26_depth_buffering.diff)

## Loading models

[Read the tutorial](https://vulkan-tutorial.com/Loading_models)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/27_model_loading.cpp)

[Go code](steps/27_model_loading/main.go)

[Diffs](diffs/27_model_loading.diff)

## Generating Mipmaps

[Read the tutorial](https://vulkan-tutorial.com/Generating_Mipmaps)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/28_mipmapping.cpp)

[Go code](steps/28_mipmapping/main.go)

[Diffs](diffs/28_mipmapping.diff)

## Multisampling

[Read the tutorial](https://vulkan-tutorial.com/Multisampling)

[Original code](https://github.com/Overv/VulkanTutorial/blob/master/code/29_multisampling.cpp)

[Go code](steps/29_multisampling/main.go)

[Diffs](diffs/29_multisampling.diff)
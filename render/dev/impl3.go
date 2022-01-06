package dev

import (
	"context"
	"github.com/deadsy/sdfx/sdf"
	"image"
	"image/color"
	"math"
	"math/rand"
	"runtime"
	"sync"
)

// CONFIGURATION

// Opt3Cam sets the default transform for the camera (pivot center, angles and distance).
func Opt3Cam(camCenter sdf.V3, pitch, yaw, dist float64) Option {
	return func(r *Renderer) {
		r.implState.CamCenter = camCenter
		r.implState.CamPitch = pitch
		r.implState.CamYaw = yaw
		r.implState.CamDist = dist
	}
}

// Opt3CamFov sets the default Field Of View for the camera (default 90º, in radians).
func Opt3CamFov(fov float64) Option {
	return func(r *Renderer) {
		r.implState.CamFOV = fov
	}
}

// RENDERER: Z is UP

type renderer3 struct {
	s          sdf.SDF3 // The SDF to render
	pixelsRand []int    // Cached set of pixels in random order to avoid shuffling (reset on recompilation and resolution changes)
}

func newDevRenderer3(s sdf.SDF3) devRendererImpl {
	r := &renderer3{
		s: s,
	}
	return r
}

func (r *renderer3) Dimensions() int {
	return 3
}

func (r *renderer3) BoundingBox() sdf.Box3 {
	return r.s.BoundingBox()
}

func (r *renderer3) Render(ctx context.Context, state *RendererState, stateLock, cachedRenderLock *sync.RWMutex, partialRender chan<- *image.RGBA, fullRender *image.RGBA) error {
	// Set all pixels to transparent initially (for partial renderings to work)
	for i := 3; i < len(fullRender.Pix); i += 4 {
		fullRender.Pix[i] = 255
	}

	// TODO: Fix blocked Render after reload

	// Update random pixels if needed
	bounds := fullRender.Bounds()
	boundsSize := sdf.V2i{bounds.Size().X, bounds.Size().Y}
	pixelCount := boundsSize[0] * boundsSize[1]
	if pixelCount != len(r.pixelsRand) {
		r.pixelsRand = make([]int, pixelCount)
		for i := 0; i < pixelCount; i++ {
			r.pixelsRand[i] = i
		}
		rand.Shuffle(len(r.pixelsRand), func(i, j int) {
			r.pixelsRand[i], r.pixelsRand[j] = r.pixelsRand[j], r.pixelsRand[i]
		})
	}

	// Spawn the workers that will render 1 pixel at a time
	jobs := make(chan *pixelRender)
	jobResults := make(chan *pixelRender)
	workerWg := &sync.WaitGroup{}
	for i := 0; i < runtime.NumCPU(); i++ {
		workerWg.Add(1)
		go func() {
			for job := range jobs {
				job.rendered = r.samplePixel(job)
				jobResults <- job
			}
			workerWg.Done()
		}()
	}
	go func() {
		workerWg.Wait()
		close(jobResults)
	}()

	// Compute camera position and main direction
	aspectRatio := float64(boundsSize[0]) / float64(boundsSize[1])
	camViewMatrix := state.Cam3MatrixNoTranslation()
	camPos := state.CamCenter.Add(camViewMatrix.MulPosition(sdf.V3{Y: -state.CamDist}))
	camDir := state.CamCenter.Sub(camPos).Normalize()
	camFovX := state.CamFOV
	camFovY := 2 * math.Atan(math.Tan(camFovX/2)*aspectRatio)
	//log.Println("cam:", camPos, "->", camDir)

	// Spawn the work generator
	go func() { // TODO: Races by reusing variables (like i in for loop)?
		// Sample each pixel on the image separately (and in random order to see the image faster)
		for _, randPixelIndex := range r.pixelsRand {
			// Sample a random pixel in the image
			sampledPixel := sdf.V2i{randPixelIndex % boundsSize[0], randPixelIndex / boundsSize[0]}
			// Queue the job for parallel processing
			jobs <- &pixelRender{
				pixel:         sampledPixel,
				bounds:        boundsSize,
				camPos:        camPos,
				camDir:        camDir,
				camViewMatrix: camViewMatrix,
				camFov:        sdf.V2{X: camFovX, Y: camFovY},
				rendered:      color.RGBA{},
			}
		}
		close(jobs) // Close the jobs channel to mark the end
	}()

	// Listen for all job results and update the image, freeing locks and sending a partial image update every batch of pixels
	const pixelBatch = 100
	pixelNum := 0
	cachedRenderLock.Lock()
	var err error
pixelLoop:
	for renderedPixel := range jobResults {
		fullRender.SetRGBA(renderedPixel.pixel[0], renderedPixel.pixel[1], renderedPixel.rendered)
		pixelNum++
		if pixelNum%pixelBatch == 0 {
			cachedRenderLock.Unlock()
			runtime.Gosched() // Breathe (let renderer do something, best-effort)
			// Check if this render is cancelled (could also check every pixel...)
			select {
			case <-ctx.Done():
				err = ctx.Err()
				break pixelLoop
			default:
			}
			// Send the partial render update
			//log.Println("Sending partial render with", pixelNum, "pixels")
			//tempFile, _ := ioutil.TempFile("", "fullRender-"+strconv.Itoa(pixelNum)+"-*.png")
			//_ = png.Encode(tempFile, fullRender)
			//log.Println("Written PNG to", tempFile.Name())
			if partialRender != nil {
				// TODO: Use a shader to fill transparent pixel with nearest neighbors to make it look better while rendering
				partialRender <- fullRender
			}
			//time.Sleep(time.Second)
			cachedRenderLock.Lock()
		}
	}
	if err == nil {
		cachedRenderLock.Unlock()
	}
	close(partialRender)
	// TODO: Draw bounding boxes over the image
	return err
}

type pixelRender struct {
	pixel, bounds  sdf.V2i // Pixel and bounds for pixel
	camPos, camDir sdf.V3  // Camera parameters
	camViewMatrix  sdf.M44 // The world to camera matrix
	camFov         sdf.V2  // Camera's field of view
	// OUTPUT
	rendered color.RGBA
}

func (r *renderer3) samplePixel(job *pixelRender) color.RGBA {
	// Generate the ray for this pixel using the given camera parameters
	rayFrom := job.camPos
	// Get pixel inside of ([-1, 1], [-1, 1])
	rayDirXZBase := job.pixel.ToV2().Div(job.bounds.ToV2()).MulScalar(2).SubScalar(1)
	// Convert to the projection over a displacement of 1
	rayDirXZBase = rayDirXZBase.Mul(sdf.V2{X: math.Tan(job.camFov.DivScalar(2).X), Y: math.Tan(job.camFov.DivScalar(2).Y)})
	rayDir := sdf.V3{X: rayDirXZBase.X, Y: 1, Z: rayDirXZBase.Y}
	// Apply the camera matrix to the default ray
	rayDir = job.camViewMatrix.MulPosition(rayDir).Normalize()
	// TODO: Orthogonal camera

	// Query the surface with the given ray
	maxRay := 10000. // TODO: Compute the actual value
	const maxSteps = 1000
	_, t, steps := sdf.Raycast3(r.s, rayFrom, rayDir, 0, 0.1, 1e-4, maxRay, maxSteps)
	//if job.pixel[0] == job.bounds[0]/2 {
	//	log.Println("ray dir:", rayDir, "T:", t, "HIT:", hit, "STEPS:", steps)
	//}

	// Convert the possible hit to a color
	if t >= 0 { // Hit the surface
		return color.RGBA{B: 255, A: 255}
	} else {
		if steps == maxSteps { // Reached the maximum amount of steps (should change parameters): fog
			return color.RGBA{R: 150, G: 50, B: 50, A: 255}
		} else { // Ray limit reached without hits: fog
			return color.RGBA{R: 150, G: 50, B: 50, A: 255}
		}
	}
}

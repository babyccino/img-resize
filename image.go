package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/h2non/bimg"
)

func getFileExtension(fileName string) (string, string, error) {
	lastPeriod := strings.LastIndex(fileName, ".")
	if lastPeriod == -1 {
		return "", "", fmt.Errorf("No extension found")
	}
	return fileName[lastPeriod:], fileName[:lastPeriod], nil
}

func resizeToWidth(img *bimg.Image, width int, outDir string, name string) {
	fmt.Println("Resizing " + name + " to " + fmt.Sprint(width) + "w")
	if img == nil {
		fmt.Fprintln(os.Stderr, "newImage is nil")
		return
	}

	size, err := img.Size()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	height := (width * size.Height) / size.Width

	buf, err := img.Resize(width, height)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	err = bimg.Write(outDir+name+"/"+fmt.Sprint(width)+"w:"+name+".webp", buf)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
}

func getStartingIndex(width int, sizes []int) int {
	for i, size := range sizes {
		if width > size {
			return i
		}
	}

	return len(sizes) - 1
}

var wg sync.WaitGroup
var tasks chan func()

func createTask(task func()) {
	wg.Add(1)
	tasks <- task
}

func startWorker() {
	for task := range tasks {
		task()
		wg.Done()
	}
}

func initWorkerPool(threadLimit int, taskBufferLength int) {
	wg = sync.WaitGroup{}
	if (taskBufferLength) == 0 {
		tasks = make(chan func())
	} else {
		tasks = make(chan func(), taskBufferLength)
	}

	if threadLimit == 0 {
		threadLimit = 4
	}

	for i := 0; i < threadLimit; i++ {
		go startWorker()
	}
}

func syncWorkerPool() {
	wg.Wait()
	close(tasks)
}

func makeResizeImageTask(buffer []byte, fileNameNoExt string, sizes []int, ch chan func(), outDir string) error {
	newImageBuf, err := bimg.NewImage(buffer).Convert(bimg.WEBP)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}

	newImage := bimg.NewImage(newImageBuf)

	dirErr := os.MkdirAll(outDir+fileNameNoExt, 0755)
	if dirErr != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}

	size, err := newImage.Size()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	startingIndex := getStartingIndex(size.Width, sizes)
	fmt.Println("Resizing " + fileNameNoExt + " to " + fmt.Sprint(sizes[startingIndex:]))
	for _, resizeWidth := range sizes[startingIndex:] {
		resizeWidthCopy := resizeWidth
		createTask(func() { resizeToWidth(newImage, resizeWidthCopy, outDir, fileNameNoExt) })
	}

	return nil
}

func getArgs() ([]int, string) {
	sizeFlag := flag.String("size", "", "comma separated list of sizes to resize to")
	outDirFlag := flag.String("outDir", "", "output directory")
	flag.Parse()

	var outDir string
	if outDirFlag == nil || len(*outDirFlag) == 0 {
		outDir = "./"
	} else {
		outDir = *outDirFlag
		if outDir[0] != '/' && outDir[0] != '.' {
			outDir = "./" + outDir
		}
		if outDir[len(outDir)-1] != '/' {
			outDir += "/"
		}
	}

	if sizeFlag == nil || len(*sizeFlag) == 0 {
		fmt.Println("No size flag provided, using default values")
		return []int{1400, 1200, 800, 400}, outDir
	}

	var sizes []int
	for _, sizeStr := range strings.Split((*sizeFlag), ",") {
		sizeNum, err := strconv.Atoi(sizeStr)
		if err != nil {
			fmt.Println("Invalid size provided, using default values")
			return []int{1400, 1200, 800, 400}, outDir
		}
		sizes = append(sizes, sizeNum)
	}

	sort.Sort(sort.Reverse(sort.IntSlice(sizes)))
	return sizes, outDir
}

func main() {
	start := time.Now()

	path, err := os.ReadDir("./")
	if err != nil {
		log.Println(err)
	}

	sizes, outDir := getArgs()

	// get only files with image extensions
	var files []os.DirEntry
	for _, file := range path {
		fileName := file.Name()
		fileExtension, _, err := getFileExtension(fileName)
		if err != nil {
			continue
		}

		if fileExtension != ".jpeg" && fileExtension != ".jpg" && fileExtension != ".png" && fileExtension != ".webp" {
			continue
		}
		files = append(files, file)
	}

	fmt.Println("Resizing " + fmt.Sprint(len(files)) + " images...")

	initWorkerPool(runtime.NumCPU(), len(files)*len(sizes))

	for _, file := range files {
		fileName := file.Name()
		createTask(func() {
			_, fileNameNoExt, err := getFileExtension(fileName)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return
			}

			buffer, err := bimg.Read(fileName)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return
			}

			makeResizeImageTask(buffer, fileNameNoExt, sizes, tasks, outDir)
		})
	}

	syncWorkerPool()

	elapsed := time.Since(start)
	log.Printf("img-resize took %s", elapsed)
	fmt.Println("Done!")
}

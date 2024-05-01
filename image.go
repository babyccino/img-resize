package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/h2non/bimg"
)

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

func initWorkerPool(threadLimit int) {
	wg = sync.WaitGroup{}
	tasks = make(chan func())

	if threadLimit == 0 {
		threadLimit = 4
	}

	for i := 0; i < threadLimit; i++ {
		go startWorker()
	}
}

func initWorkerPoolBuffer(threadLimit int, taskBufferLength int) {
	wg = sync.WaitGroup{}
	if (taskBufferLength) <= 0 {
		panic("task buffer length must be greater than 0")
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

func getFileExtension(fileName string) (extension string, fileNameNoExt string, err error) {
	lastPeriod := strings.LastIndex(fileName, ".")
	if lastPeriod == -1 {
		return "", "", fmt.Errorf("No extension found")
	}

	lastSlash := strings.LastIndex(fileName, "/")
	if lastSlash == -1 {
		fileNameNoExt = fileName[:lastPeriod]
	} else {
		fileNameNoExt = fileName[lastSlash+1 : lastPeriod]
	}

	return fileName[lastPeriod:], fileNameNoExt, nil
}

func parseInputDir(inputDirFlag *string) string {
	if inputDirFlag == nil || len(*inputDirFlag) == 0 {
		return "."
	}

	inputDir := *inputDirFlag
	if inputDir == "/" {
		return ""
	}
	if inputDir == "." {
		return "."
	}
	if inputDir == ".." {
		return ".."
	}

	if inputDir[0] != '/' && inputDir[0] != '.' && inputDir[0] != '~' {
		inputDir = "./" + inputDir
	}
	if inputDir[len(inputDir)-1] == '/' {
		inputDir = inputDir[:len(inputDir)-1]
	}
	return inputDir
}

// inputDir does not end with a slash
func getArgs() (file bool, filePath, outDir, inputDir string, sizes []int, recursive bool, container bool) {
	sizeFlag := flag.String("size", "", "comma separated list of sizes to resize to\nex: size=100,200,300\n")
	outDirFlag := flag.String("outDir", "", "output directory\nex: outDir=./output/\n")
	inputDirFlag := flag.String("inputDir", "", "input directory\nex: inputDir=./images/\n")
	fileFlag := flag.String("file", "", "file name\nwill override inputDir and recursive flags\nex: file=./images/image.jpg\n")
	recursiveFlag := flag.Bool("r", false, "recursively search for images in input directory\n")
	containerFlag := flag.Bool("c", false, "puts all resized images in folders of the same name as the original image\n")
	flag.Parse()

	if containerFlag == nil {
		container = false
	} else {
		container = *containerFlag
	}

	if sizeFlag == nil || len(*sizeFlag) == 0 {
		fmt.Println("No size flag provided, using default values")
		sizes = []int{1400, 1200, 800, 400}
		return file, filePath, outDir, inputDir, sizes, recursive, container
	}

	for _, sizeStr := range strings.Split((*sizeFlag), ",") {
		sizeNum, err := strconv.Atoi(sizeStr)
		if err != nil {
			fmt.Println("Invalid size provided, using default values")
			sizes = []int{1400, 1200, 800, 400}
			return file, filePath, outDir, inputDir, sizes, recursive, container
		}
		sizes = append(sizes, sizeNum)
		sort.Sort(sort.Reverse(sort.IntSlice(sizes)))
	}

	if fileFlag == nil || len(*fileFlag) == 0 {
		file = false
	} else {
		file = true
		filePath = *fileFlag
		return file, filePath, outDir, inputDir, sizes, recursive, container
	}

	if recursiveFlag == nil {
		recursive = false
	} else {
		recursive = *recursiveFlag
	}

	outDir = parseInputDir(outDirFlag)
	inputDir = parseInputDir(inputDirFlag)

	return file, filePath, outDir, inputDir, sizes, recursive, container
}

var exceptedExtensions = []string{".jpeg", ".jpg", ".png", ".webp"}

func resizeToWidth(img *bimg.Image, width int, outDir string, fileName string) {
	log.Printf("Resizing %s to %d w", fileName, width)
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

	outPath := fmt.Sprintf("%s/%s?w=%d.webp", outDir, fileName, width)
	err = bimg.Write(outPath, buf)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
}

func makeResizeImageTask(filePath string, outDir string, fileName string, sizes []int, isWebp bool) {
	buffer, err := bimg.Read(filePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	newImageBuf, err := bimg.NewImage(buffer).Convert(bimg.WEBP)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	newImage := bimg.NewImage(newImageBuf)

	dirErr := os.MkdirAll(outDir+"/"+fileName, 0755)
	if dirErr != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	// create full size webp image if the original image is not webp
	outDir = fmt.Sprintf("%s/%s", outDir, fileName)
	if !isWebp {
		outFileName := fmt.Sprintf("%s.webp", outDir)
		err = bimg.Write(outFileName, newImageBuf)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
	}

	size, err := newImage.Size()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	startingIndex := getStartingIndex(size.Width, sizes)
	fmt.Println("Resizing " + fileName + " to " + fmt.Sprint(sizes[startingIndex:]))
	for _, resizeWidth := range sizes[startingIndex:] {
		resizeWidthCopy := resizeWidth
		createTask(func() { resizeToWidth(newImage, resizeWidthCopy, outDir, fileName) })
	}
}

func resizeInPath(currDir string, outDir string, sizes []int, recursive bool, container bool) {
	files, err := os.ReadDir(currDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	for _, file := range files {
		filePath := file.Name()
		if recursive && file.Type().IsDir() {
			resizeInPath(currDir+"/"+filePath, outDir+"/"+filePath, sizes, recursive, container)
			continue
		}

		fileExtension, fileName, err := getFileExtension(filePath)
		if err != nil {
			continue
		}

		if !slices.Contains(exceptedExtensions, fileExtension) {
			continue
		}

		fullFilePath := currDir + "/" + file.Name()
		log.Printf("Resizing %s to %s and saving to %s %s\n", fullFilePath, fmt.Sprint(sizes), outDir, fileName)
		createTask(func() { makeResizeImageTask(fullFilePath, outDir, fileName, sizes, fileExtension == ".webp") })
	}
}

func main() {
	start := time.Now()

	file, filePath, outDir, inputDir, sizes, recursive, container := getArgs()

	if file {
		fileExtension, fileName, err := getFileExtension(filePath)
		if err != nil {
			return
		}

		if !slices.Contains(exceptedExtensions, fileExtension) {
			return
		}

		log.Printf("Resizing %s to %s and saving to %s %s\n", filePath, fmt.Sprint(sizes), outDir, fileName)

		initWorkerPool(runtime.NumCPU())

		makeResizeImageTask(filePath, outDir, fileName, sizes, fileExtension == ".webp")

		syncWorkerPool()
		return
	}

	initWorkerPool(runtime.NumCPU())

	resizeInPath(inputDir, outDir, sizes, recursive, container)

	syncWorkerPool()

	elapsed := time.Since(start)
	log.Printf("img-resize took %s\n", elapsed)
	fmt.Println("Done!")
}

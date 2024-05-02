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

var exceptedExtensions = []string{".jpeg", ".jpg", ".png", ".webp"}

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

type Args struct {
	file      bool
	filePath  string
	outDir    string
	inputDir  string
	sizes     []int
	recursive bool
	container bool
}

// inputDir does not end with a slash
func getArgs() Args {
	sizeFlag := flag.String("size", "", "comma separated list of sizes to resize to\nex: size=100,200,300\n")
	outDirFlag := flag.String("outDir", "", "output directory\nex: outDir=./output/\n")
	inputDirFlag := flag.String("inputDir", "", "input directory\nex: inputDir=./images/\n")
	fileFlag := flag.String("file", "", "file name\nwill override inputDir and recursive flags\nex: file=./images/image.jpg\n")
	recursiveFlag := flag.Bool("r", false, "recursively search for images in input directory\n")
	containerFlag := flag.Bool("c", false, "puts all resized images in folders of the same name as the original image\n")
	flag.Parse()

	var container bool
	if containerFlag == nil {
		container = false
	} else {
		container = *containerFlag
	}

	var sizes []int
	if sizeFlag == nil || len(*sizeFlag) == 0 {
		log.Println("No size flag provided, using default values")
		sizes = []int{1400, 1200, 800, 400}
	} else {
		for _, sizeStr := range strings.Split((*sizeFlag), ",") {
			sizeNum, err := strconv.Atoi(sizeStr)
			if err != nil {
				log.Println("There was an error parsing the sizes argument, using default values")
				sizes = []int{1400, 1200, 800, 400}
				break
			}
			sizes = append(sizes, sizeNum)
			sort.Sort(sort.Reverse(sort.IntSlice(sizes)))
		}
	}

	outDir := parseInputDir(outDirFlag)

	var file bool
	if fileFlag == nil || len(*fileFlag) == 0 {
		file = false
	} else {
		file = true
		filePath := *fileFlag
		return Args{
			file:      file,
			filePath:  filePath,
			outDir:    outDir,
			inputDir:  "",
			sizes:     sizes,
			recursive: false,
			container: container,
		}
	}

	var recursive bool
	if recursiveFlag == nil {
		recursive = false
	} else {
		recursive = *recursiveFlag
	}

	inputDir := parseInputDir(inputDirFlag)

	return Args{
		file:      file,
		filePath:  "",
		outDir:    outDir,
		inputDir:  inputDir,
		sizes:     sizes,
		recursive: recursive,
		container: container,
	}
}

type Resizer struct {
	sizes             []int
	img               *bimg.Image
	outDir            string
	fileName          string
	originalExtension string
	imgSize           bimg.ImageSize
}

func (self *Resizer) Init(fullFilePath string) error {
	fileExtension, fileName, err := getFileExtension(fullFilePath)
	if err != nil {
		return err
	}

	if !slices.Contains(exceptedExtensions, fileExtension) {
		return fmt.Errorf("Invalid file extension")
	}
	self.fileName = fileName
	self.originalExtension = fileExtension

	buffer, err := bimg.Read(fullFilePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}

	newImageBuf, err := bimg.NewImage(buffer).Convert(bimg.WEBP)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}

	self.img = bimg.NewImage(newImageBuf)

	size, err := self.img.Size()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	self.imgSize = size

	return nil
}

func (resizer *Resizer) CreateResizeTasks() {
	if !(resizer.originalExtension == ".webp") {
		createTask(func() { resizer.writeFullSize() })
	}

	startingIndex := getStartingIndex(resizer.imgSize.Width, resizer.sizes)
	log.Printf("Resizing %s to %v", resizer.fileName, resizer.sizes[startingIndex:])
	for _, resizeWidth := range resizer.sizes[startingIndex:] {
		// not needed after as of 1.22
		resizeWidthCopy := resizeWidth
		createTask(func() { resizer.ResizeToWidth(resizeWidthCopy) })
	}
}

func (self *Resizer) ResizeToWidth(width int) {
	log.Printf("Resizing %s to %d w", self.fileName, width)

	size, err := self.img.Size()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	height := (width * size.Height) / size.Width

	buf, err := self.img.Resize(width, height)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	outPath := fmt.Sprintf("%s/%s?w=%d.webp", self.outDir, self.fileName, width)
	err = bimg.Write(outPath, buf)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
}

func (resizer *Resizer) Containerise() {
	resizer.outDir = fmt.Sprintf("%s/%s", resizer.outDir, resizer.fileName)
	dirErr := os.MkdirAll(resizer.outDir, 0755)
	if dirErr != nil {
		fmt.Fprintln(os.Stderr, dirErr)
		return
	}
}

func (resizer *Resizer) writeFullSize() error {
	outFileName := fmt.Sprintf("%s/%s.webp", resizer.fileName, resizer.outDir)
	err := bimg.Write(outFileName, resizer.img.Image())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	return nil
}

func resizeAllInPath(currDir string, outDir string, sizes []int, recursive bool, container bool) {
	files, err := os.ReadDir(currDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	for _, file := range files {
		filePath := file.Name()
		if recursive && file.Type().IsDir() {
			resizeAllInPath(currDir+"/"+filePath, outDir+"/"+filePath, sizes, recursive, container)
			continue
		}

		fullFilePath := currDir + "/" + file.Name()
		log.Printf("Resizing %s to %s and saving to %s\n", fullFilePath, fmt.Sprint(sizes), outDir)
		createTask(func() {
			resizer := Resizer{
				sizes:  sizes,
				outDir: outDir,
			}

			resizer.Init(fullFilePath)
			if container {
				resizer.Containerise()
			}
			// create full size webp image if the original image is not webp
			resizer.CreateResizeTasks()
		})
	}
}

func main() {
	start := time.Now()

	args := getArgs()

	initWorkerPool(runtime.NumCPU())

	if args.file {
		resizer := Resizer{
			sizes:  args.sizes,
			outDir: args.outDir,
		}

		resizer.Init(args.filePath)
		if args.container {
			resizer.Containerise()
		}
		// create full size webp image if the original image is not webp
		resizer.CreateResizeTasks()
	} else {
		resizeAllInPath(args.inputDir, args.outDir, args.sizes, args.recursive, args.container)
	}

	syncWorkerPool()

	elapsed := time.Since(start)
	log.Printf("img-resize took %s\n", elapsed)
	fmt.Println("Done!")
}

package main

import (
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
	flag "github.com/spf13/pflag"
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

type Program struct {
	singleFile bool
	outDir     string
	inputDir   string
	sizes      []int
	recursive  bool
	container  bool
	errC       chan error
}

// inputDir does not end with a slash
func InitProgram() Program {
	sizeFlag := flag.StringP("size", "s", "", "comma separated list of sizes to resize to\nex: size=100,200,300\n")
	outDirFlag := flag.StringP("outDir", "o", "", "output directory\nex: outDir=./output/\n")
	inputDirFlag := flag.StringP("inputDir", "i", "", "input directory\nex: inputDir=./images/\n")
	fileFlag := flag.StringP("file", "f", "", "file name\nwill override inputDir and recursive flags\nex: file=./images/image.jpg\n")
	recursiveFlag := flag.BoolP("recursive", "r", false, "recursively search for images in input directory\n")
	containerFlag := flag.BoolP("container", "c", false, "puts all resized images in folders of the same name as the original image\n")
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
		return Program{
			singleFile: file,
			inputDir:   filePath,
			outDir:     outDir,
			sizes:      sizes,
			recursive:  false,
			container:  container,
			errC:       make(chan error),
		}
	}

	var recursive bool
	if recursiveFlag == nil {
		recursive = false
	} else {
		recursive = *recursiveFlag
	}

	inputDir := parseInputDir(inputDirFlag)

	return Program{
		singleFile: file,
		outDir:     outDir,
		inputDir:   inputDir,
		sizes:      sizes,
		recursive:  recursive,
		container:  container,
		errC:       make(chan error),
	}
}

func (program *Program) ResizeFile(fileName string, subDir *string) error {
	var outDir string
	var fullFilePath string
	if subDir == nil {
		outDir = program.outDir
		fullFilePath = program.inputDir + "/" + fileName
	} else {
		outDir = program.outDir + "/" + *subDir
		fullFilePath = program.inputDir + "/" + *subDir + "/" + fileName
	}

	resizer := Resizer{
		sizes:  program.sizes,
		outDir: outDir,
		errC:   program.errC,
	}

	err := resizer.Init(fullFilePath)
	if err != nil {
		return err
	}
	if program.container {
		err := resizer.Containerise()
		if err != nil {
			return err
		}
	}
	// create full size webp image if the original image is not webp
	resizer.CreateResizeTasks()
	return nil
}

func (program *Program) resizeAllInPath(subDir *string) error {
	var dir string
	if subDir == nil {
		dir = program.inputDir
	} else {
		dir = program.inputDir + "/" + *subDir
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		program.errC <- err
		return err
	}

	for _, file := range files {
		filePath := file.Name()
		if program.recursive && file.Type().IsDir() {
			var newDir string
			if subDir == nil {
				newDir = filePath
			} else {
				newDir = *subDir + "/" + filePath
			}
			program.resizeAllInPath(&newDir)
		} else {
			createTask(func() {
				program.ResizeFile(filePath, subDir)
			})
		}

	}
	return nil
}

func (program *Program) Work() error {
	if program.singleFile {
		resizer := Resizer{
			sizes:  program.sizes,
			outDir: program.outDir,
			errC:   program.errC,
		}

		err := resizer.Init(program.inputDir)
		if err != nil {
			return err
		}
		if program.container {
			err := resizer.Containerise()
			if err != nil {
				return err
			}
		}
		// create full size webp image if the original image is not webp
		resizer.CreateResizeTasks()
	} else {
		err := program.resizeAllInPath(nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func (program *Program) Shutdown() {
	close(program.errC)
}

type Resizer struct {
	sizes             []int
	img               *bimg.Image
	outDir            string
	fileName          string
	originalExtension string
	imgSize           bimg.ImageSize

	errC chan error
}

func (resizer *Resizer) Init(fullFilePath string) error {
	fileExtension, fileName, err := getFileExtension(fullFilePath)
	if err != nil {
		resizer.errC <- err
		return err
	}

	if !slices.Contains(exceptedExtensions, fileExtension) {
		err := fmt.Errorf("Invalid file extension")
		resizer.errC <- err
		return err
	}
	resizer.fileName = fileName
	resizer.originalExtension = fileExtension

	buffer, err := bimg.Read(fullFilePath)
	if err != nil {
		resizer.errC <- err
		return err
	}

	newImageBuf, err := bimg.NewImage(buffer).Convert(bimg.WEBP)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}

	resizer.img = bimg.NewImage(newImageBuf)

	size, err := resizer.img.Size()
	if err != nil {
		resizer.errC <- err
		return err
	}
	resizer.imgSize = size
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

func (resizer *Resizer) ResizeToWidth(width int) error {
	log.Printf("Resizing %s to %d w", resizer.fileName, width)

	size, err := resizer.img.Size()
	if err != nil {
		resizer.errC <- err
		return err
	}

	height := (width * size.Height) / size.Width

	buf, err := resizer.img.Resize(width, height)
	if err != nil {
		resizer.errC <- err
		return err
	}

	outPath := fmt.Sprintf("%s/%s?w=%d.webp", resizer.outDir, resizer.fileName, width)
	err = bimg.Write(outPath, buf)
	if err != nil {
		resizer.errC <- err
		return err
	}
	return nil
}

func (resizer *Resizer) Containerise() error {
	resizer.outDir = fmt.Sprintf("%s/%s", resizer.outDir, resizer.fileName)
	err := os.MkdirAll(resizer.outDir, 0755)
	if err != nil {
		resizer.errC <- err
		return err
	}
	return nil
}

func (resizer *Resizer) writeFullSize() error {
	outFileName := fmt.Sprintf("%s/%s.webp", resizer.fileName, resizer.outDir)
	err := bimg.Write(outFileName, resizer.img.Image())
	if err != nil {
		resizer.errC <- err
		return err
	}
	return nil
}

func main() {
	start := time.Now()

	program := InitProgram()

	go func() {
		for err := range program.errC {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	initWorkerPool(runtime.NumCPU())
	program.Work()
	syncWorkerPool()
	program.Shutdown()

	elapsed := time.Since(start)
	log.Printf("img-resize took %s\n", elapsed)
	fmt.Println("Done!")
}

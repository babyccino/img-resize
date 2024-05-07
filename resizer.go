package main

import (
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/h2non/bimg"
)

type Resizer struct {
	sizes             []int
	img               *bimg.Image
	outDir            string
	fileName          string
	originalExtension string
	format            string
	imgSize           bimg.ImageSize

	errC chan error
}

func NewResizer(
	sizes []int,
	outDir string,
	errC chan error,
	format string,
	fullFilePath string,
	container bool,
) (*Resizer, error) {
	fileExtension, fileName, err := getFileExtension(fullFilePath)
	if err != nil {
		errC <- err
		return nil, err
	}

	if !slices.Contains(exceptedExtensions, fileExtension) {
		err := fmt.Errorf("Invalid file extension")
		errC <- err
		return nil, err
	}

	buffer, err := bimg.Read(fullFilePath)
	if err != nil {
		errC <- err
		return nil, err
	}

	newImageBuf, err := bimg.NewImage(buffer).Convert(bimg.WEBP)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return nil, err
	}

	img := bimg.NewImage(newImageBuf)

	size, err := img.Size()
	if err != nil {
		errC <- err
		return nil, err
	}
	imgSize := size

	if container {
		outDir = fmt.Sprintf("%s/%s", outDir, fileName)
		err := os.MkdirAll(outDir, 0755)
		if err != nil {
			errC <- err
			return nil, err
		}
	}

	format = fmt.Sprintf("%s/%s", outDir, format)

	return &Resizer{
		sizes:             sizes,
		img:               img,
		outDir:            outDir,
		fileName:          fileName,
		originalExtension: fileExtension,
		format:            format,
		imgSize:           imgSize,
		errC:              errC,
	}, nil
}

func (resizer *Resizer) FormatFileName(size int) string {
	replacer := strings.NewReplacer("{f}", resizer.fileName, "{s}", strconv.Itoa(size))
	return replacer.Replace(resizer.format)
}

func (resizer *Resizer) FormatFileNameFullSize() string {
	return fmt.Sprintf("%s/%s.webp", resizer.fileName, resizer.outDir)}

func (resizer *Resizer) CreateResizeTasks() {
	if !(resizer.originalExtension == ".webp") {
		createTask(func() { resizer.WriteFullSize() })
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

	fileName := resizer.FormatFileName(width)
	err = bimg.Write(fileName, buf)
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

func (resizer *Resizer) WriteFullSize() error {
	fileName := resizer.FormatFileNameFullSize()
	err := bimg.Write(fileName, resizer.img.Image())
	if err != nil {
		resizer.errC <- err
		return err
	}
	return nil
}

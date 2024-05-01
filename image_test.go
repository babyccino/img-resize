package main

import "testing"

func TestParseInputDir(test *testing.T) {
	test.Parallel()

	test.Run("parseInputDir", func(test *testing.T) {
		test.Parallel()

		actual := parseInputDir(nil)
		expected := "."
		if actual != expected {
			test.Errorf("expected: %s, actual: %s", expected, actual)
		}

		testValues := func(input, expected string) {
			actual := parseInputDir(&input)
			if actual != expected {
				test.Errorf("expected: %s, actual: %s", expected, actual)
			}
		}

		testValues("", ".")
		testValues("/", "")
		testValues("~", "~")
		testValues("/abc", "/abc")
		testValues("/abc/def", "/abc/def")
		testValues("~/abc/def", "~/abc/def")
		testValues("./abc/def", "./abc/def")
		testValues("abc/def", "./abc/def")
		testValues("abc", "./abc")
		testValues("..", "..")
		testValues("../hi", "../hi")
		testValues("./../hi", "./../hi")
	})

	test.Run("getFileExtension", func(test *testing.T) {
		test.Parallel()

		// (fileName string) (extension string, fileNameNoExt string, err error)
		testValues := func(input, expectedExtension, expectedFileName string, expectedErr bool) {
			extension, fileName, err := getFileExtension(input)
			if extension != expectedExtension || fileName != expectedFileName || (err != nil) != expectedErr {
				test.Errorf("expected: %s, %s, %v, actual: %s, %s, %v", expectedExtension, expectedFileName, expectedErr, extension, fileName, err)
			}
		}

		testValues("hi.txt", ".txt", "hi", false)
		testValues("abc/hi.png", ".png", "hi", false)
		testValues("~/hello/abc/there.jpeg", ".jpeg", "there", false)
	})
}

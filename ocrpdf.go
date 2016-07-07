/*
Copyright (c) 2016, Maxim Konakov
All rights reserved.

Redistribution and use in source and binary forms, with or without modification,
are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice,
   this list of conditions and the following disclaimer.
2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.
3. Neither the name of the copyright holder nor the names of its contributors
   may be used to endorse or promote products derived from this software without
   specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED.
IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT,
INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING,
BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY
OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING
NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE,
EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

package main

import (
	"bytes"
	"container/heap"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

var firstPage, lastPage uint
var inputFileName, language string

func main() {
	// command line parameters
	flag.UintVar(&firstPage, "first", 1, "First page number")
	flag.UintVar(&lastPage, "last", 0, "Last page number")
	flag.StringVar(&language, "lang", "eng", "Document language")
	flag.Parse()

	switch flag.NArg() {
	case 0:
		die("Input file is not specified")
	case 1:
		inputFileName = flag.Arg(0)
	default:
		die("Too many input files")
	}

	if err := run(); err != nil {
		die(err.Error())
	}
}

func run() error {
	// temporary directory
	dir, err := ioutil.TempDir("", "ocr-")
	if err != nil {
		return err
	}

	dir = filepath.FromSlash(dir + "/") // make sure we have trailing slash
	defer os.RemoveAll(dir)

	// signal processing
	signals := make(chan os.Signal, 5)

	go func() {
		<-signals
		os.RemoveAll(dir)
		die("Interrupted")
	}()

	signal.Notify(signals, os.Interrupt, os.Kill)

	// extract images from input file
	if err = extractImages(dir); err != nil {
		return err
	}

	// OCR
	return ocr(dir, func(text []byte) error {
		_, e := os.Stdout.Write(append(text, '\n'))
		return e
	})
}

// 'pdfimages' driver
func extractImages(dir string) error {
	// "$extractor" $from $to "$input" $tmp_dir/
	args := []string{"-f", strconv.Itoa(int(firstPage))}

	if lastPage > firstPage {
		args = append(args, "-l", strconv.Itoa(int(lastPage)))
	}

	args = append(args, inputFileName, dir)

	var msg bytes.Buffer

	cmd := exec.Command("pdfimages", args...)
	cmd.Stderr = &msg
	err := cmd.Run()

	if err != nil {
		if _, ok := err.(*exec.ExitError); ok && msg.Len() > 0 {
			err = errors.New(msg.String())
		}
	}

	return err
}

// request/response data structures for parallel ocr
type ocrRequest struct {
	no    uint
	image string
}

type ocrResult struct {
	req  ocrRequest
	err  error
	warn string
	text bytes.Buffer
}

// heap of ocrResult structures for restoring the original page order
type resultHeap []*ocrResult

func (h resultHeap) Len() int           { return len(h) }
func (h resultHeap) Less(i, j int) bool { return h[i].req.no < h[j].req.no }
func (h resultHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *resultHeap) Push(x interface{}) { *h = append(*h, x.(*ocrResult)) }

func (h *resultHeap) Pop() interface{} {
	n := len(*h) - 1
	val := (*h)[n]
	*h = (*h)[:n]
	return val
}

// OCR driver
func ocr(dir string, f func([]byte) error) error {
	// list all image files
	files, err := filepath.Glob(dir + "*.pbm")
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return errors.New("No images found in file " + inputFileName)
	}

	if len(files) > 1 {
		sort.Strings(files)
	}

	// channels
	n := runtime.NumCPU()
	results := make(chan *ocrResult, n)
	requests := make(chan *ocrRequest, len(files))
	var wg sync.WaitGroup

	// workers
	for i := 0; i < n; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			var msg bytes.Buffer

			for req := range requests {
				resp := &ocrResult{
					req: *req,
				}

				// "$ocr" "$image" - $lang
				cmd := exec.Command("tesseract", req.image, "-", "-l", language)
				cmd.Stdout = &resp.text
				cmd.Stderr = &msg

				if resp.err = cmd.Run(); resp.err != nil {
					if _, ok := resp.err.(*exec.ExitError); ok && msg.Len() > 0 {
						resp.err = errors.New(errMsg(&msg, req.no))
					}
				} else if msg.Len() > 0 {
					resp.warn = errMsg(&msg, req.no)
				}

				results <- resp
				msg.Reset()
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// fill in request channel
	for i, file := range files {
		requests <- &ocrRequest{uint(i), file}
	}

	close(requests)

	// read results
	var h resultHeap
	i := uint(0)

	heap.Init(&h)

	for r := range results {
		if r.err != nil {
			return r.err
		}

		heap.Push(&h, r)

		for ; len(h) > 0 && h[0].req.no == i; i++ {
			r := heap.Pop(&h).(*ocrResult)

			if len(r.warn) > 0 {
				fmt.Fprintln(os.Stderr, "WARNING:", r.warn)
			}

			// process the result
			for s, _ := r.text.ReadBytes('\n'); len(s) > 0; s, _ = r.text.ReadBytes('\n') {
				if err = f(bytes.TrimRightFunc(s, unicode.IsSpace)); err != nil {
					return err
				}
			}
		}
	}

	if h.Len() > 0 {
		panic(fmt.Sprintf("Heap still has %d elements", h.Len()))
	}

	return nil
}

// little helper functions
func errMsg(buff *bytes.Buffer, reqNo uint) string {
	s, _ := buff.ReadString('\n')
	return fmt.Sprintf("(page %d) %s", reqNo+firstPage, strings.TrimSpace(s))
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, "ERROR:", msg)
	os.Exit(1)
}

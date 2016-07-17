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

var cmd *cmdLineOptions

func main() {
	var err error

	// command line parameters
	if cmd, err = parseCmdLine(); err != nil {
		die(err.Error())
	}

	// read filters
	lineFilter, textFilter, err := makeFilters()

	if err != nil {
		die(err.Error())
	}

	// OCR
	var text bytes.Buffer

	if err = extractText(&text, lineFilter); err != nil {
		die(err.Error())
	}

	// apply full-text filter
	if len(cmd.output) == 0 {
		_, err = os.Stdout.Write(textFilter(text.Bytes()))
	} else {
		var out *os.File

		if out, err = os.OpenFile(cmd.output, os.O_WRONLY|os.O_TRUNC, 0666); err == nil {
			defer func() {
				if err == nil {
					err = out.Close()
				} else {
					out.Close()
				}
			}()

			_, err = out.Write(textFilter(text.Bytes()))
		}
	}

	if err != nil {
		die(err.Error())
	}
}

func extractText(text *bytes.Buffer, filter func([]byte) []byte) (err error) {
	// temporary directory
	var dir string

	dir, err = ioutil.TempDir("", "ocr-")
	if err != nil {
		return
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
		return
	}

	// OCR
	return ocr(dir, text, filter)
}

// 'pdfimages' driver
func extractImages(dir string) error {
	args := []string{"-tiff", "-f", strconv.Itoa(int(cmd.first))}

	if cmd.last >= cmd.first {
		args = append(args, "-l", strconv.Itoa(int(cmd.last)))
	}

	args = append(args, cmd.input, dir)

	var msg bytes.Buffer

	cmd := exec.Command("pdfimages", args...)
	cmd.Stderr = &msg
	err := cmd.Run()

	if err != nil {
		if _, ok := err.(*exec.ExitError); ok && msg.Len() > 0 {
			s := msg.String()

			if strings.HasPrefix(s, "pdfimages") { // got 'usage' string instead of an error message
				s = "Program 'pdfimages' exited with an error; parameters: " + strings.Join(args, " ")
			}

			err = errors.New(s)
		}
	}

	return err
}

// request/response data structures for parallel ocr
type ocrRequest struct {
	no    uint
	image string
}

func (req *ocrRequest) process() (text []byte, err error) {
	text, err = exec.Command("tesseract", req.image, "-", "-l", cmd.language).Output()

	if err != nil {
		msg := fmt.Sprintf("(page %d) ", req.no+cmd.first)

		if e, ok := err.(*exec.ExitError); ok {
			if n := bytes.IndexByte(e.Stderr, '\n'); n >= 0 { // get first line only
				e.Stderr = e.Stderr[:n]
			}

			msg += string(bytes.TrimSpace(e.Stderr))
		} else {
			msg += err.Error()
		}

		err = errors.New(msg)
	}

	return
}

type ocrResult struct {
	req  ocrRequest
	err  error
	text []byte
}

func processOCRRequest(req *ocrRequest) (r *ocrResult) {
	r = &ocrResult{req: *req}
	r.text, r.err = req.process()
	return
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
func ocr(dir string, text *bytes.Buffer, filter func([]byte) []byte) error {
	// list all image files
	files, err := filepath.Glob(dir + "*.tif")
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return errors.New("No images found in file " + cmd.input)
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

			for req := range requests {
				results <- processOCRRequest(req)
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
		heap.Push(&h, r)

		for ; len(h) > 0 && h[0].req.no == i; i++ {
			r = heap.Pop(&h).(*ocrResult)

			if r.err != nil {
				return r.err
			}

			// process the result
			reader := bytes.NewBuffer(r.text)

			for s, _ := reader.ReadBytes('\n'); len(s) > 0; s, _ = reader.ReadBytes('\n') {
				if _, err := text.Write(filter(bytes.TrimRightFunc(s, unicode.IsSpace))); err != nil {
					return err
				}

				if err := text.WriteByte('\n'); err != nil {
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

// little helpers
func die(msg string) {
	fmt.Fprintln(os.Stderr, "ERROR:", msg)
	os.Exit(1)
}

// fiter function maker
func makeFilters() (lineFilter, textFilter func([]byte) []byte, err error) {
	rules := new(ruleList)

	for _, name := range cmd.filters {
		var file *os.File

		if file, err = os.Open(name); err != nil {
			return
		}

		defer file.Close()

		if err = rules.add(file, name); err != nil {
			return
		}
	}

	lineFilter = seqFilter(rules.lineRules)
	textFilter = seqFilter(rules.textRules)
	return
}

func seqFilter(rules []func([]byte) []byte) func([]byte) []byte {
	if len(rules) == 0 {
		return func(s []byte) []byte { return s }
	}

	return func(s []byte) []byte {
		if len(s) > 0 {
			for _, f := range rules {
				if s = f(s); len(s) == 0 {
					break
				}
			}
		}

		return s
	}
}

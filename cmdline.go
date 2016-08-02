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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const (
	usageFmt = `Usage: %s [OPTION]... FILE
Extract text from a scanned pdf or djvu document FILE.

Options:
  -f,--first N        first page number (optional, default: 1)
  -l,--last  N        last page number (optional, default: last page of the document)
  -F,--filter FILE    filter specification file name (optional, may be given multiple times)
  -L,--language LANG  document language (optional, default: 'eng')
  -o,--output FILE    output file name (optional, default: stdout)
  -h,--help           display this help and exit
  -v,--version        output version information and exit
`

	version  = "0.4.2"
	maxPages = 3000
)

// command line arguments
type cmdLineOptions struct {
	first, last             uint
	input, output, language string
	filters                 []string
}

func parseCmdLine() (cmd *cmdLineOptions, err error) {
	if len(os.Args) == 1 {
		showUsageAndExit()
	}

	cmd = &cmdLineOptions{
		first:    1,
		language: "eng",
	}

	args := argReader(os.Args[1:])

	for opt := args.next(); len(opt) > 0; opt = args.next() {
		switch opt {
		case "-f", "--first":
			err = setUint(&cmd.first, args.next(), "--first", maxPages)
		case "-l", "--last":
			err = setUint(&cmd.last, args.next(), "--last", maxPages)
		case "-L", "--language":
			err = cmd.setLanguage(args.next())
		case "-F", "--filter":
			err = cmd.addFilter(args.next())
		case "-o", "--output":
			if cmd.output = args.next(); len(cmd.output) == 0 {
				err = errors.New("Missing output file name")
			}
		case "-h", "--help":
			showUsageAndExit()
		case "-v", "--version":
			fmt.Println("ver.", version)
			os.Exit(1)
		default:
			if len(args.next()) > 0 {
				err = errors.New("Unknown command line switch: " + opt)
			} else {
				cmd.input = opt
			}
		}

		if err != nil {
			return
		}
	}

	if len(cmd.input) == 0 {
		err = errors.New("Input file is not specified")
	}

	return
}

func setUint(target *uint, s, opt string, max uint) error {
	if len(s) == 0 {
		return fmt.Errorf("Missing argument for the option \"%s\"", opt)
	}

	val, err := strconv.Atoi(s)

	if err != nil {
		return fmt.Errorf("Invalid argument \"%s\" for the option \"%s\"", s, opt)
	}

	if val <= 0 || uint(val) > max {
		return fmt.Errorf("Invalid argument \"%d\" for the option \"%s\": impossible value", val, opt)
	}

	*target = uint(val)
	return nil
}

func (cmd *cmdLineOptions) setLanguage(s string) error {
	if len(s) == 0 {
		return errors.New("Missing argument for option \"--language\"")
	}

	cmd.language = s
	return nil
}

func (cmd *cmdLineOptions) addFilter(s string) error {
	if len(s) == 0 {
		return errors.New("Missing argument for option \"--filter\"")
	}

	if info, err := os.Stat(s); err != nil {
		if os.IsNotExist(err) {
			return err
		}

		return err
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("\"%s\" is not a file", s)
	}

	cmd.filters = append(cmd.filters, s)
	return nil
}

// argument strings reader
type argReader []string

func (args *argReader) next() (res string) {
	if len(*args) > 0 {
		res, *args = (*args)[0], (*args)[1:]
	}

	return
}

// "usage" display
func showUsageAndExit() {
	fmt.Fprintf(os.Stderr, usageFmt, filepath.Base(os.Args[0]))
	os.Exit(1)
}

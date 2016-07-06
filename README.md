# OCRPDF

### Motivation
Once I had a task of OCR'ing a number of scanned documents. Each document was in .pdf format and
contained multiple scanned images. Given the number of files to process I decided to develop
a program to automate this. I am leaving the source code of the program here in the hope that
someone may find it useful.

### Compilation
From the directory of the project do:
```
go build -o ocrpdf ocrpdf.go
```

This creates executable file `ocrpdf`.

### Invocation
Command line options:
```
ocrpdf [-first n] [-last n] [-lang xxx] input-file.pdf
```

where `-first` and `-last` specify the range of pages to process, and the `-lang` argument specifies the language(s)
of the document (default `eng`) and gets passed directly to `tesseract` tool. All these parameters are optional.

The program always directs its output to stdout.

For example, to process a document from page 12 to page 26 in Russian:
```
./ocrpdf -first 12 -last 26 -lang rus some.pdf > document.txt
```

### Setup
Internally the program relies on the tool `pdfimages` to extract images from the input file, and on `tesseract`
program to do the actual image to text conversion. The former tool is usually a part of `poppler-utils` package,
while the latter is included in `tesseract-ocr` package. By default, the `tesseract` tool comes with the English
language support only, other languages should be installed separately, for example, run `sudo apt install tesseract-ocr-rus`
to install the Russian language support. To find out what languages are currently installed type
`tesseract --list-langs`.

### Details
The tool first runs the `pdfimages` program to extract images to a temporary directory, and then invokes
`tesseract` on each image in parallel, assembling the output in the original order and directing it to stdout.
The resulting text has any trailing white-space stripped, otherwise there is no processing done to it. Sometimes,
the output contains long runs of empty lines, which can usually be normalised by piping the text through `cat -s`.

In general, removing all the artefacts produced by OCR is a difficult task. The project includes a Python script,
`filter`, which removes some artefacts from a simple Russian text without any rich formatting, tables, formulas, etc.
The result is usually less than ideal, it depends on the original text and the quality of the OCR process,
so don't expect it to replace a human editor. The script takes its input from stdin and produces output to stdout.


###### Lisence: BSD

###### Platform: Linux (tested on Linux Mint 18 64bit)


BIN = ocrpdf
SRC = ocrpdf.go filters.go

.PHONY : debug release clean

release : GOFLAGS = -ldflags "-s"

debug release : $(BIN)

$(BIN) : $(SRC)
	go build -o $@ $(GOFLAGS) $^

clean :
	rm -f $(BIN)

BIN = go-ocr
SRC = ocr.go filters.go cmdline.go

.PHONY : debug release clean

release : GOFLAGS = -ldflags="-s -w"

debug release : $(BIN)

$(BIN) : $(SRC)
	go build -o $@ $(GOFLAGS) $^

clean :
	rm -f $(BIN)

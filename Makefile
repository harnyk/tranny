BINARY := tranny
INSTALL_DIR := $(GOPATH)/bin

.PHONY: build install clean

build:
	go build -o $(BINARY) .

install:
	go install .

clean:
	rm -f $(BINARY)

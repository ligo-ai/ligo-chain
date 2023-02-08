BUILD_FLAGS = -tags "$(BUILD_TAGS)" -ldflags "

build:
	@ echo "Building client..."
	@ go build -o $(GOPATH)/bin/ligochain ./chain/ligochain/
	@ echo "Done building."
#.PHONY: ligochain
ligochain:
	@ echo "Building client..."
	@ go build -o $(GOPATH)/bin/ligochain ./chain/ligochain/
	@ echo "Done building."
	@ echo "Run ./ligochain to launch the client. "

install:
	@ echo "Installing..."
	@ go install -mod=readonly $(BUILD_FLAGS) ./chain/ligochain
	@ echo "Install success."
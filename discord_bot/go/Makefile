.PHONY: build clean

BINARY   := bootstrap
ZIP_NAME := bonfire_discord_bot.zip

build:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(BINARY) .
	cd .. && zip -j $(ZIP_NAME) go/$(BINARY) 2>/dev/null || \
	  python3 -c "import zipfile,os; z=zipfile.ZipFile('$(ZIP_NAME)','w',zipfile.ZIP_DEFLATED); z.write('go/$(BINARY)','$(BINARY)'); z.close()"
	rm -f $(BINARY)

clean:
	rm -f $(BINARY) ../$(ZIP_NAME)

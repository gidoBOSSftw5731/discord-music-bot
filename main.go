package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gidoBOSSftw5731/log"
)

const (
	tmpdir       = "/tmp/JAMusicBot"
	subdir       = "audios"
	viddir       = "videos"
	youtubeChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"
)

func main() {
	setup()
	err := dlToTmp("FdnLtHEeqtU")
	if err != nil {
		log.Fatalln("Failed to get video info", err)
	}
}

func dlToTmp(url string) error {

	//make slice of ID  for file saving purposes
	idSplit := strings.Split(url, "")

	// make file
	file, err := os.Create(filepath.Join(tmpdir, viddir, idSplit[0], idSplit[1], url) + ".mp4")
	if err != nil {
		return err
	}
	defer file.Close()

	return nil
}

func setup() {
	// setup log func (stupid ik but its here)
	log.EnableLevel("info")
	log.EnableLevel("error")
	log.EnableLevel("debug")
	log.EnableLevel("trace")

	// make dir

	// make subdirs
	ytCharsSplit := strings.Split(youtubeChars, "")
	for f := 0; f < len(youtubeChars); f++ {
		for s := 0; s < len(youtubeChars); s++ {
			err := os.MkdirAll(filepath.Join(tmpdir, subdir, ytCharsSplit[f], ytCharsSplit[s]), 755)
			err = os.MkdirAll(filepath.Join(tmpdir, viddir, ytCharsSplit[f], ytCharsSplit[s]), 755)
			if err != nil {
				log.Fatalln("Error in making subdirectories", err)
			}

		}
	}
}

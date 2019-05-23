package main

import (
	"os"

	"github.com/gidoBOSSftw5731/log"
	"github.com/rylio/ytdl"
)

const tmpdir = "/tmp/JAMusicBot"

func main() {
	err := dlToTmp("https://www.youtube.com/watch?v=FdnLtHEeqtU")
	if err != nil {
		log.Fatalln("Failed to get video info", err)
	}
}

func dlToTmp(url string) error {
	// make video object
	vid, err := ytdl.GetVideoInfo(url)
	if err != nil {
		return err
	}

	// make file
	file, err := os.Create(tmpdir + vid.Title + ".mp3")
	if err != nil {
		return err
	}
	defer file.Close()

	// download to file
	vid.Download(vid.Formats[0], file)
	return nil
}

func setup() {
	// setup log func (stupid ik but its here)
	log.EnableLevel("info")
	log.EnableLevel("error")
	log.EnableLevel("debug")
	log.EnableLevel("trace")

	os.Mkdir(tmpdir, 0640) // make dir
}

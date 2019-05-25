package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gidoBOSSftw5731/log"
	"github.com/giorgisio/goav/avformat"
	"github.com/rylio/ytdl"
)

const (
	tmpdir       = "/tmp/JAMusicBot"
	subdir       = "audios"
	viddir       = "videos"
	youtubeChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"
)

func main() {
	setup()
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

	//make slice of ID  for file saving purposes
	idSplit := strings.Split(vid.ID, "")

	// make file
	file, err := os.Create(filepath.Join(tmpdir, viddir, idSplit[0], idSplit[1], vid.ID) + ".mp4")
	if err != nil {
		return err
	}
	defer file.Close()

	// download to temp file
	vid.Download(vid.Formats[172], file)

	// now time to rip the audio
	finalFile, err := os.Create(filepath.Join(tmpdir, subdir, idSplit[0], idSplit[1], vid.ID) + ".mp3")
	if err != nil {
		return err
	}
	defer finalFile.Close()

	// Register all formats and codecs
	avformat.AvRegisterAll()

	ctx := avformat.AvformatAllocContext()

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

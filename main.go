package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/BrianAllred/goydl"
	"github.com/gidoBOSSftw5731/log"
	"github.com/jinzhu/configor"
)

const (
	subdir = "audios"
	//viddir       = "videos" // unneeded due to move to ydl
	youtubeChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"
)

var (
	tmpdir string
	Config = struct {
		GoogleDeveloperKey string `required:"true"`
		DiscordBotToken    string `required:"true"`
	}{}
)

func main() {
	setup()
	err := dlToTmp("CxQKltWI0NA", tmpdir)
	if err != nil {
		log.Fatalln("Failed to get video info", err)
	}
}

func dlToTmp(url, tmpdir string) error {

	// open youtubedl client
	youtubeDl := goydl.NewYoutubeDl()

	//make slice of ID  for file saving purposes
	idSplit := strings.Split(url, "")

	log.Traceln(filepath.Join(tmpdir, subdir, idSplit[0], idSplit[1], fmt.Sprintf("%v.mp3", url)))

	// set options
	youtubeDl.Options.Output.Value = filepath.Join(tmpdir, subdir, idSplit[0], idSplit[1], fmt.Sprintf("%v.mp3", url))
	youtubeDl.Options.ExtractAudio.Value = true
	youtubeDl.Options.AudioFormat.Value = "mp3"
	youtubeDl.VideoURL = fmt.Sprintf("www.youtube.com/watch?v=%v", url)
	// listen to errors from ydl
	//go io.Copy(os.Stdout, youtubeDl.Stdout)
	//go io.Copy(os.Stderr, youtubeDl.Stderr)

	dwnld, err := youtubeDl.Download()
	if err != nil {
		return err
	}
	dwnld.Wait()

	return nil
}

func setup() {
	log.SetCallDepth(4)

	err := configor.Load(&Config, "config.yml")
	if err != nil {
		log.Fatalf("Error with config: %v", err)
	}

	//var err error
	tmpdir, err = ioutil.TempDir("/tmp", "JAMB")
	if err != nil {
		log.Fatalln("Failed to get video info", err)
	}

	// make dir

	// make subdirs
	ytCharsSplit := strings.Split(youtubeChars, "")
	for f := 0; f < len(youtubeChars); f++ {
		for s := 0; s < len(youtubeChars); s++ {
			err := os.MkdirAll(filepath.Join(tmpdir, subdir, ytCharsSplit[f], ytCharsSplit[s]), 0755)
			//err = os.MkdirAll(filepath.Join(tmpdir, viddir, ytCharsSplit[f], ytCharsSplit[s]), 755) // unneeded due to move to ydl
			if err != nil {
				log.Fatalln("Error in making subdirectories", err)
			}

		}
	}
}

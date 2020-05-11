package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/api/option"

	"github.com/BrianAllred/goydl"
	"github.com/bwmarrin/dgvoice"
	"github.com/bwmarrin/discordgo"
	"github.com/gidoBOSSftw5731/log"
	"github.com/jinzhu/configor"
	"google.golang.org/api/youtube/v3"
)

const (
	subdir = "audios"
	//viddir       = "videos" // unneeded due to move to ydl
	youtubeChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"
)

var Config = struct {
	GoogleDeveloperKey string `required:"true" json:"googleDeveloperKey"`
	DiscordBotToken    string `required:"true" json:"discordBotToken"`
	Prefix             string `default:"**" json:"prefix"`
	googleDeveloperKey string
	prefix             string
	discordBotToken    string
}{}

var (
	tmpdir        string
	connectionMap map[string]*discordgo.VoiceConnection
)

func main() {
	setup()
	discordStart()
}

//for legacy support, please dont rely on this
// It's 4am, I woke up at 10pm, I really cba to change all the calls
// to this damn function
func errCheck(msg string, err error) {
	if err != nil {
		log.Fatalf("%s: %+v", msg, err)
	}
}

func discordStart() {
	discord, err := discordgo.New("Bot " + Config.discordBotToken)
	errCheck("error creating discord session", err)

	discord.AddHandler(commandHandler)
	discord.AddHandler(func(discord *discordgo.Session, ready *discordgo.Ready) {
		servers := discord.State.Guilds

		err = discord.UpdateStatus(2, fmt.Sprintf("Pin all the things! | %vhelp | Pinning in %v servers!",
			Config.prefix, len(servers)))
		if err != nil {
			log.Errorln("Error attempting to set my status")
		}

		log.Debugf("PinnerBoi has started on %d servers", len(servers))
	})

	err = discord.Open()
	errCheck("Error opening connection to Discord", err)
	defer discord.Close()

	<-make(chan struct{})
}

func findUserVoiceState(session *discordgo.Session, userid string) (*discordgo.VoiceState, error) {
	for _, guild := range session.State.Guilds {
		for _, vs := range guild.VoiceStates {
			if vs.UserID == userid {
				return vs, nil
			}
		}
	}
	return nil, fmt.Errorf("could not find user's voice state")
}

func commandHandler(discord *discordgo.Session, message *discordgo.MessageCreate) {
	if message.Content == "" || len(message.Content) < len(Config.prefix) {
		return
	}
	if message.Content[:len(Config.Prefix)] != Config.Prefix ||
		len(strings.Split(message.Content, Config.prefix)) < 2 {
		return
	}

	log.Debugln("prefix found")

	command := strings.Split(message.Content, Config.Prefix)[1]
	commandContents := strings.Split(message.Content, " ") // 0 = *command, 1 = first arg, etc

	log.Tracef("Command: %v, command contents %v", command, commandContents)

	if len(command) < 2 {
		log.Errorln("No command sent")
		return
	}

	switch strings.Split(command, " ")[0] {
	case "p", "play", "Play", "song", "P":
		searchQuery := strings.Join(commandContents[1:], " ")

		log.Debugf("Searching for %v", searchQuery)

		video, err := searchForVideo(searchQuery)
		if err != nil {
			discord.ChannelMessageSend(message.ChannelID,
				fmt.Sprintf("Error finding video: %v", err))
			return
		}

		fpath, err := dlToTmp(video.Id.VideoId)
		if err != nil {
			discord.ChannelMessageSend(message.ChannelID,
				fmt.Sprintf("Error saving video: %v", err))
			return
		}

		log.Debugf("Path for song: %v", fpath)

		discord.ChannelMessageSend(fmt.Sprintf("Playing %v now!", video.Snippet.Title),
			message.ChannelID)

		//connect to voice channel

		vs, err := findUserVoiceState(discord, message.Author.ID)
		if err != nil {
			log.Errorln(err)
			discord.ChannelMessageSend(message.ChannelID, "Join a voice channel")
			return
		}

		dgv, err := discord.ChannelVoiceJoin(vs.GuildID,
			vs.ChannelID, false, true)
		if err != nil {
			discord.ChannelMessageSend(message.ChannelID,
				fmt.Sprintf("Error playing video: %v", err))
			return
		}

		dgvoice.PlayAudioFile(dgv, fpath, make(chan bool))
	}
}

func searchForVideo(input string) (*youtube.SearchResult, error) {

	ctx := context.Background()
	service, err := youtube.NewService(ctx, option.WithAPIKey(Config.googleDeveloperKey))
	if err != nil {
		return nil, err
	}

	// Make the API call to YouTube.
	call := service.Search.List("id,snippet").
		Q(input).
		MaxResults(3)
	response, err := call.Do()
	if err != nil {
		return nil, err
	}

	if len(response.Items) == 0 {
		return nil, fmt.Errorf("no results")
	}

	for _, i := range response.Items {
		if i.Id.Kind == "youtube#video" {
			return i, nil
		}
	}

	return nil, fmt.Errorf("no videos in query")
}

func dlToTmp(url string) (string, error) {

	// open youtubedl client
	youtubeDl := goydl.NewYoutubeDl()

	//make slice of ID  for file saving purposes
	idSplit := strings.Split(url, "")

	fpath := filepath.Join(tmpdir, subdir, idSplit[0], idSplit[1], fmt.Sprintf("%v.mp3", url))
	log.Traceln(fpath)

	if _, err := os.Stat(fpath); err == nil {
		log.Traceln("already downloaded")
		return fpath, nil
	}
	// set options
	youtubeDl.Options.Output.Value = filepath.Join(tmpdir, subdir, idSplit[0], idSplit[1], fmt.Sprintf("%v.mp3", url))
	youtubeDl.Options.ExtractAudio.Value = true
	youtubeDl.Options.AudioFormat.Value = "mp3"
	youtubeDl.Options.KeepVideo = goydl.BoolOption{Value: false} // why is this a thing

	youtubeDl.VideoURL = fmt.Sprintf("www.youtube.com/watch?v=%v", url)
	// listen to errors from ydl
	//go io.Copy(os.Stdout, youtubeDl.Stdout)
	//go io.Copy(os.Stderr, youtubeDl.Stderr)

	dwnld, err := youtubeDl.Download()
	if err != nil {
		return "", err
	}
	dwnld.Wait()

	return fpath, nil
}

func setup() {
	log.SetCallDepth(4)

	err := configor.Load(&Config, "config.json")
	if err != nil {
		log.Fatalf("Error with config: %v", err)
	}

	Config.googleDeveloperKey = Config.GoogleDeveloperKey
	Config.prefix = Config.Prefix
	Config.discordBotToken = Config.DiscordBotToken

	//println(Config.googleDeveloperKey)

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

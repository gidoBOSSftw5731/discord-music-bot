package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	tmpdir       string
	playingMap   = make(map[string]bool)
	queue        = make(map[string][]string)
	connMap      = make(map[string]*discordgo.VoiceConnection)
	loopMap      = make(map[string]bool)
	loopQueueMap = make(map[string]bool)
	// youtubeSearchCache takes a youtube search and returns its search results
	youtubeSearchCache = make(map[string]*youtube.SearchResult)
	// ytdlCache takes a path to a downloaded video and returns it's youtube search results
	ytdlCache = make(map[string]*youtube.SearchResult)
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

		err = discord.UpdateStatus(2, fmt.Sprintf(
			"It might not be good, but it's mine| %vhelp | Jamming in %v servers!",
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

func removeFromSlice(slice []string, s int) []string {
	if len(slice) == 1 {
		var newSlice []string
		return newSlice
	}
	return append(slice[:s], slice[s+1:]...)
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

	switch strings.Split(command, " ")[0] {
	case "p", "play", "Play", "song", "P":
		if len(command) < 2 {
			log.Errorln("No command sent")
			return
		}
		msg, _ := discord.ChannelMessageSend(message.ChannelID,
			"Please wait while I download the song")

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

		ytdlCache[fpath] = video

		log.Debugf("Path for song: %v", fpath)

		//connect to voice channel
		discord.ChannelMessageDelete(msg.ChannelID, msg.ID)

		isPlayingInServer := playingMap[message.GuildID]
		switch isPlayingInServer {
		case true:
			queue[message.GuildID] = append(queue[message.GuildID], fpath)
		case false:
			loopMap[message.GuildID] = false
			loopQueueMap[message.GuildID] = false

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

			connMap[vs.GuildID] = dgv

			playingMap[vs.GuildID] = true

			queue[vs.GuildID] = []string{fpath}

			for playingMap[vs.GuildID] {
				if len(queue[vs.GuildID]) != 0 {
					fpath = queue[vs.GuildID][0]
					if !loopMap[message.GuildID] || !loopQueueMap[message.GuildID] {
						discord.ChannelMessageSend(message.ChannelID, fmt.Sprintf("Playing \"%v\" now! http://youtu.be/%v",
							ytdlCache[fpath].Snippet.Title, ytdlCache[fpath].Id.VideoId))
					}
					dgvoice.PlayAudioFile(dgv, fpath, make(chan bool))
					if !loopMap[vs.GuildID] && !loopQueueMap[vs.GuildID] {
						queue[vs.GuildID] = removeFromSlice(queue[vs.GuildID], 0)
					} else if !loopQueueMap[vs.GuildID] {
						queue[vs.GuildID] = append(removeFromSlice(queue[vs.GuildID], 0),
							fpath)
					}
				} else { // yes I am using else, sue me
					playingMap[vs.GuildID] = false
					dgv.Disconnect()
					//dgv.Unlock()
				}
			}

		}

	case "q", "queue", "Q", "Queue":
		var fields []*discordgo.MessageEmbedField
		for n, i := range queue[message.GuildID] {
			v := ytdlCache[i]
			f := discordgo.MessageEmbedField{Name: string(n), Inline: false,
				Value: v.Snippet.Title}

			fields = append(fields, &f)
		}

		discord.ChannelMessageSendEmbed(message.ChannelID, &discordgo.MessageEmbed{
			Title:     "Queue for server:",
			Author:    &discordgo.MessageEmbedAuthor{},
			Color:     rand.Intn(16777215), // Green
			Fields:    fields,
			Timestamp: time.Now().Format(time.RFC3339)}) // Discord wants ISO8601; RFC3339 is an extension of ISO8601 and should be completely compatible.

	case "help", "h":
		fields := []*discordgo.MessageEmbedField{
			&discordgo.MessageEmbedField{
				Name:  "Play a song",
				Value: "p: Play song, either provide title, youtube ID, or youtube URL."},
			&discordgo.MessageEmbedField{
				Name:  "Show server queue",
				Value: "q: List the queue for the server"},
			&discordgo.MessageEmbedField{
				Name:  "Leave the voice channel",
				Value: "d: Self explainatory"},
			&discordgo.MessageEmbedField{
				Name:  "Loop one track",
				Value: "loop: Self explainatory, overrides loopqueue"},
			&discordgo.MessageEmbedField{
				Name:  "Loop queue",
				Value: "loop: Self explainatory"},
			&discordgo.MessageEmbedField{
				Name:  "Invite this bot to other servers",
				Value: "Invite URL: https://discord.com/api/oauth2/authorize?client_id=581249727958351891&permissions=37054784&scope=bot"}}

		discord.ChannelMessageSendEmbed(message.ChannelID, &discordgo.MessageEmbed{
			Title:       "How to use:",
			Description: fmt.Sprintf("All commands must be prefixed by the bot prefix: %v", Config.Prefix),
			Author:      &discordgo.MessageEmbedAuthor{},
			Color:       rand.Intn(16777215), // Green
			Fields:      fields,
			Timestamp:   time.Now().Format(time.RFC3339)}) // Discord wants ISO8601; RFC3339 is an extension of ISO8601 and should be completely compatible.
	case "leave", "disconnect", "d", "dc", "die":
		if val, ok := connMap[message.GuildID]; ok &&
			playingMap[message.GuildID] {
			queue[message.GuildID] = []string{}
			playingMap[message.GuildID] = false
			val.Disconnect()
			discord.ChannelMessageSend(message.ChannelID, "Leaving")
		} else {
			discord.ChannelMessageSend(message.ChannelID, "Not in a channel :(")
		}
	case "loop", "Loop", "singleloop":
		loopMap[message.GuildID] = !loopMap[message.GuildID]

		discord.ChannelMessageSend(message.ChannelID,
			fmt.Sprintf("Looping one track/song is now set to %v", loopMap[message.GuildID]))
	case "loopqueue", "lq", "Loopqueue":
		loopQueueMap[message.GuildID] = !loopQueueMap[message.GuildID]

		discord.ChannelMessageSend(message.ChannelID,
			fmt.Sprintf("Looping queue is now set to %v", loopQueueMap[message.GuildID]))
	}
}

func searchForVideo(input string) (*youtube.SearchResult, error) {

	if val, ok := youtubeSearchCache[input]; ok {
		log.Traceln("Getting search from cache (yay!)")
		return val, nil
	}

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
			youtubeSearchCache[input] = i
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

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/option"

	"github.com/BrianAllred/goydl"
	"github.com/bwmarrin/discordgo"
	"github.com/gidoBOSSftw5731/dgvoice"
	"github.com/gidoBOSSftw5731/log"
	"github.com/jinzhu/configor"
	"google.golang.org/api/youtube/v3"
)

const (
	subdir = "audios"
	//viddir       = "videos" // unneeded due to move to ydl
	youtubeChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"
	// my user ID (dont @ me) for admin commands like ban
	botOwner = "181965297249550336"
)

//Lyrics parses the json for the lyrics
type Lyrics struct {
	Title     string `json:"title"`
	Author    string `json:"author"`
	Lyrics    string `json:"lyrics"`
	Thumbnail struct {
		Genius string `json:"genius"`
	} `json:"thumbnail"`
	Links struct {
		Genius string `json:"genius"`
	} `json:"links"`
	Error string `json:"error",omitempty`
}

var Config = struct {
	GoogleDeveloperKey string `required:"true" json:"googleDeveloperKey"`
	DiscordBotToken    string `required:"true" json:"discordBotToken"`
	Prefix             string `default:"*" json:"prefix"`
	//GeniusToken        string `default:"" json:"geniusToken"`
	googleDeveloperKey string
	prefix             string
	discordBotToken    string
}{}

var (
	tmpdir          string
	lyricHTTPClient = http.Client{Timeout: 5 * time.Second}
	playingMap      = make(map[string]bool)
	queue           = make(map[string][]string)
	// banList is made to allow users to be banned by the owner of the bot
	// all this does is takes the guild ID and their ID and stores it
	banList      = make(map[string][]string)
	connMap      = make(map[string]*discordgo.VoiceConnection)
	loopMap      = make(map[string]bool)
	loopQueueMap = make(map[string]bool)
	stopMap      = make(map[string]chan bool)
	startTimeMap = make(map[string]time.Time)
	// if a server is paused or skipped
	pausedMap = make(map[string]bool)
	// youtubeSearchCache takes a youtube search and returns its search results
	youtubeSearchCache = make(map[string]*youtube.VideoListResponse)
	// ytdlCache takes a path to a downloaded video and returns it's youtube search results
	ytdlCache = make(map[string]*youtube.VideoListResponse)
	// This is a mess, thanks eyecatchUp on Stackoverflow for doing this
	// https://stackoverflow.com/questions/2964678/jquery-youtube-url-validation-with-regex/10315969#10315969
	youtubeURLRegex = regexp.MustCompile(
		`(?:https?:\/\/)?(?:www\.)?(?:youtu\.be\/|youtube\.com\/(?:embed\/|v\/|watch\?v=|watch\?.+&v=))((\w|-){11})(?:\S+)?`)
	helpMessageFields = []*discordgo.MessageEmbedField{
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
			Value: "loopqueue: Self explainatory"},
		&discordgo.MessageEmbedField{
			Name:  "Remove duplicate tracks",
			Value: "removedupes: removes duplicate songs, the first one in the queue stays."},
		&discordgo.MessageEmbedField{
			Name:  "Shuffle",
			Value: "shuffle: mixes tracks randomly,  does not follow looping and may cause unexpected issues while looping."},
		&discordgo.MessageEmbedField{
			Name:  "Skip the current song",
			Value: "s: Skips the current song, does NOT remove from queue, cannot resume"},
		&discordgo.MessageEmbedField{
			Name:  "Remove a song",
			Value: "remove: Removes the song input in the same order as is in the queue. will not skip if it is the current song"},
		&discordgo.MessageEmbedField{
			Name:  "Now Playing",
			Value: "np: Self explainatory"},
		&discordgo.MessageEmbedField{
			Name:  "Play next",
			Value: "playnext: (alias playtop) play a song next, dont skip to it though"},
		&discordgo.MessageEmbedField{
			Name:  "Get Lyrics",
			Value: "lyrics: Get's lyrics from \"The Internet\" for either the current song playing, or the name of the song if provided in the command"},
		&discordgo.MessageEmbedField{
			Name:  "Extra commands",
			Value: "If there is a command not listed here, check the rythm help list, some commands are unfinished or just for testing: https://rythmbot.co/features#list"},
		&discordgo.MessageEmbedField{
			Name:  "Invite this bot to other servers",
			Value: "Invite URL: https://discord.com/api/oauth2/authorize?client_id=581249727958351891&permissions=37054784&scope=bot"},
		&discordgo.MessageEmbedField{
			Name:  "See the source code",
			Value: "Github URL: https://imagen.click/d/jamb_git"}}
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

		err = discord.UpdateGameStatus(2, fmt.Sprintf(
			"It might not be good, but it's mine| %vhelp | Jamming in %v servers!",
			Config.prefix, len(servers)))
		if err != nil {
			log.Errorln("Error attempting to set my status")
		}

		log.Debugf("JAMB has started on %d servers", len(servers))
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
	if len(slice) < 2 {
		var newSlice []string
		return newSlice
	}
	return append(slice[:s], slice[s+1:]...)
}

func commandPlay(discord *discordgo.Session, message *discordgo.MessageCreate,
	command string, commandContents []string, addtotop bool) {
	if len(command) < 2 {
		log.Errorln("No command sent")
		return
	}
	msg, _ := discord.ChannelMessageSend(message.ChannelID,
		"Please wait while I download the song, please note downloading a large playlist may take a long time.")

	searchQuery := strings.Join(commandContents[1:], " ")
	log.Debugf("Searching for %v", searchQuery)

	switch {
	case (len(commandContents[1:]) == 1) && strings.Contains(searchQuery, "playlist?list="):
		fpaths, err := returnPlaylist(searchQuery)
		if err != nil {
			discord.ChannelMessageSend(message.ChannelID,
				fmt.Sprintf("Error getting playlist: %v", err))
			return
		}

		if queue[message.GuildID] == nil {
			queue[message.GuildID] = []string{}
		}

		switch {
		case !addtotop || len(queue[message.GuildID]) == 0:
			queue[message.GuildID] = append(queue[message.GuildID], fpaths...)
		case addtotop:
			// this is probably the worst way to do this, but it's what I have to do
			queue[message.GuildID] = append([]string{queue[message.GuildID][0]},
				append(fpaths, queue[message.GuildID][1:]...)...)
		}
	default:
		video, err := searchForVideo(searchQuery)
		if err != nil {
			discord.ChannelMessageSend(message.ChannelID,
				fmt.Sprintf("Error finding video: %v", err))
			return
		}

		fpath, err := dlToTmp(video.Items[0].Id)
		if err != nil {
			discord.ChannelMessageSend(message.ChannelID,
				fmt.Sprintf("Error saving video: %v", err))
			return
		}

		log.Debugf("Path for song: %v", fpath)
		ytdlCache[fpath] = video
		if queue[message.GuildID] == nil {
			queue[message.GuildID] = []string{}
		}

		switch {
		case !addtotop || len(queue[message.GuildID]) == 0:
			queue[message.GuildID] = append(queue[message.GuildID], fpath)
		case addtotop:
			queue[message.GuildID] = append([]string{queue[message.GuildID][0], fpath},
				queue[message.GuildID][1:]...)
		}
	}

	//connect to voice channel
	discord.ChannelMessageDelete(msg.ChannelID, msg.ID)

	isPlayingInServer := playingMap[message.GuildID]
	startTimeMap[message.GuildID] = time.Now()
	// this should be an if statement since I no longer have a true
	switch isPlayingInServer {
	case false:
		loopMap[message.GuildID] = false
		loopQueueMap[message.GuildID] = false
		attemptedtoleave := false

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

		//queue[vs.GuildID] = []string{fpath}
		starttime := float64(0)

	PlayingLoop:
		for playingMap[vs.GuildID] {

			if len(queue[vs.GuildID]) != 0 {
				attemptedtoleave = false
				fpath := queue[vs.GuildID][0]
				if !loopMap[message.GuildID] && !loopQueueMap[message.GuildID] {
					discord.ChannelMessageSend(message.ChannelID, fmt.Sprintf("Playing \"%v\" now! http://youtu.be/%v",
						ytdlCache[fpath].Items[0].Snippet.Title, ytdlCache[fpath].Items[0].Id))
				}

				if time.Now().Add(10 * time.Hour).Before(startTimeMap[message.GuildID]) {
					discord.ChannelMessageSend(message.ChannelID,
						"Leaving since it has been 10 hours.")
					break
				}

				log.Traceln("Starting Song at ", starttime)
				stopMap[vs.GuildID] = make(chan bool)
				endTime := make(chan float64)
				dgvoice.PlayAudioFile(dgv, fpath, stopMap[vs.GuildID], endTime, starttime, false)

				if pausedMap[vs.GuildID] {
					select {
					case currenttime, ok := <-endTime:
						if !ok {
							continue
						}
						currenttime += starttime
						log.Traceln("Pausing Song at ", currenttime)
						discord.ChannelMessageSend(message.ChannelID,
							fmt.Sprintf("Paused at %v", currenttime))
						pausedMap[vs.GuildID] = false
						// wait for resume
						<-stopMap[vs.GuildID]
						starttime = currenttime
						log.Traceln("Resetting loop to:", starttime)
						continue PlayingLoop
					default:
					}
				}
				if !loopMap[vs.GuildID] && !loopQueueMap[vs.GuildID] {
					queue[vs.GuildID] = removeFromSlice(queue[vs.GuildID], 0)
				} else if loopQueueMap[vs.GuildID] {
					queue[vs.GuildID] = append(removeFromSlice(queue[vs.GuildID], 0),
						fpath)

				}
				starttime = 0
			} else { // yes I am using else, sue me
				if !attemptedtoleave {
					attemptedtoleave = true
					time.Sleep(time.Second * 5)
					break
				}
			}
		}
		playingMap[vs.GuildID] = false
		// clear queue for safety
		queue[vs.GuildID] = []string{}
		if dgv == nil {
			break
		}

		dgv.Disconnect()
		//dgv.Unlock()

	}

}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func commandHandler(discord *discordgo.Session, message *discordgo.MessageCreate) {
	if message.Content == "" || len(message.Content) < len(Config.prefix) ||
		stringInSlice(message.Author.ID, banList[message.GuildID]) {
		return
	}
	//log.Tracef("UserID: %v, is banned: %v, banlist: %v",
	//	message.Author.ID, stringInSlice(message.Author.ID, banList[message.GuildID]), banList[message.GuildID])
	if message.Content[:len(Config.Prefix)] != Config.Prefix ||
		len(strings.Split(message.Content, Config.prefix)) < 2 {
		return
	}

	log.Debugln("prefix found")

	// false = normal true = debug
	if false && message.Author.ID != botOwner {
		log.Debugln("Debug mode enabled, owner only can use")
		return
	}

	command := strings.Split(message.Content, Config.Prefix)[1]
	commandContents := strings.Split(message.Content, " ") // 0 = *command, 1 = first arg, etc

	log.Tracef("Command: %v, command contents %v", command, commandContents)

	switch strings.Split(command, " ")[0] {
	case "p", "play", "Play", "song", "P":
		commandPlay(discord, message, command, commandContents, false)
	case "q", "queue", "Q", "Queue":
		var fields []*discordgo.MessageEmbedField
		for n, i := range queue[message.GuildID] {
			v := ytdlCache[i]
			f := discordgo.MessageEmbedField{Name: strconv.Itoa(n), Inline: false,
				Value: v.Items[0].Snippet.Title}

			fields = append(fields, &f)
		}

		discord.ChannelMessageSendEmbed(message.ChannelID, &discordgo.MessageEmbed{
			Title:     "Queue for server:",
			Author:    &discordgo.MessageEmbedAuthor{},
			Color:     rand.Intn(16777215), // Green
			Fields:    fields,
			Timestamp: time.Now().Format(time.RFC3339)}) // Discord wants ISO8601; RFC3339 is an extension of ISO8601 and should be completely compatible.

	case "help", "h":

		discord.ChannelMessageSendEmbed(message.ChannelID, &discordgo.MessageEmbed{
			Title:       "How to use:",
			Description: fmt.Sprintf("All commands must be prefixed by the bot prefix: %v", Config.Prefix),
			Author:      &discordgo.MessageEmbedAuthor{},
			Color:       rand.Intn(16777215), // random (I know this says green somewhere, it isnt)
			Fields:      helpMessageFields,
			Timestamp:   time.Now().Format(time.RFC3339)}) // Discord wants ISO8601; RFC3339 is an extension of ISO8601 and should be completely compatible.
	case "leave", "disconnect", "d", "dc", "die":
		if val, ok := connMap[message.GuildID]; ok &&
			playingMap[message.GuildID] {
			discord.ChannelMessageSend(message.ChannelID, "Leaving")
			queue[message.GuildID] = []string{}
			playingMap[message.GuildID] = false
			val.Disconnect()
			stopMap[message.GuildID] <- true
		} else {
			discord.ChannelMessageSend(message.ChannelID, "Not in a channel :(")
		}
	case "loop", "Loop", "singleloop", "l":
		loopMap[message.GuildID] = !loopMap[message.GuildID]

		discord.ChannelMessageSend(message.ChannelID,
			fmt.Sprintf("Looping one track/song is now set to %v", loopMap[message.GuildID]))
	case "loopqueue", "lq", "Loopqueue":
		loopQueueMap[message.GuildID] = !loopQueueMap[message.GuildID]

		discord.ChannelMessageSend(message.ChannelID,
			fmt.Sprintf("Looping queue is now set to %v", loopQueueMap[message.GuildID]))
	case "shuffle":
		if len(queue[message.GuildID]) > 1 {
			rand.Shuffle(len(queue[message.GuildID]), func(i, j int) {
				queue[message.GuildID][i], queue[message.GuildID][j] = queue[message.GuildID][j], queue[message.GuildID][i]
			})
		}
		discord.ChannelMessageSend(message.ChannelID, "Shuffling")
	case "skip", "s", "S":
		stopMap[message.GuildID] <- true
		discord.ChannelMessageSend(message.ChannelID, "skipped")
	case "remove", "rm":
		if len(commandContents) != 2 {
			log.Errorln("No command sent")
			return
		}
		n, err := strconv.Atoi(commandContents[1])
		if err != nil {
			log.Errorln(err)
			return
		}
		if n >= len(queue[message.GuildID]) || n < 0 {
			discord.ChannelMessageSend(message.ChannelID, "Out of range")
			return
		}
		t := queue[message.GuildID][n]
		queue[message.GuildID] = removeFromSlice(queue[message.GuildID], n)
		discord.ChannelMessageSend(message.ChannelID, fmt.Sprintf("Removed \"%v\"", ytdlCache[t].Items[0].Snippet.Title))
	case "removedupes":
		if queue[message.GuildID] == nil {
			discord.ChannelMessageSend(message.ChannelID, "Queue empty")
		}

		queue[message.GuildID] = unique(queue[message.GuildID])

		discord.ChannelMessageSend(message.ChannelID, "Removed dupes")
	case "np", "nowplaying", "whatsplaying", "playing", "whatisupmydude":
		switch {
		case queue[message.GuildID] == nil:
			discord.ChannelMessageSend(message.ChannelID, "No queue for this server")
		case len(queue[message.GuildID]) == 0:
			discord.ChannelMessageSend(message.ChannelID, "Nothing in the queue")
		default:
			np := ytdlCache[queue[message.GuildID][0]]
			// there definitely could never be bad data, it could never happen
			ptime, _ := time.Parse(time.RFC3339, np.Items[0].Snippet.PublishedAt)
			fields := []*discordgo.MessageEmbedField{
				&discordgo.MessageEmbedField{
					Name:  "Publishing time",
					Value: ptime.Format(time.RFC1123),
				}}

			var thumbnailURL string
			//apparently some thumbnails are nil, this is dumb
			for _, i := range []*youtube.Thumbnail{np.Items[0].Snippet.Thumbnails.Maxres,
				np.Items[0].Snippet.Thumbnails.High, np.Items[0].Snippet.Thumbnails.Medium,
				np.Items[0].Snippet.Thumbnails.Standard, np.Items[0].Snippet.Thumbnails.Default} {
				if i != nil {
					thumbnailURL = i.Url
					break
				}
			}

			log.Debugf("Thumbnail: %v", thumbnailURL)
			discord.ChannelMessageSendEmbed(message.ChannelID, &discordgo.MessageEmbed{
				Title:       np.Items[0].Snippet.Title,
				Image:       &discordgo.MessageEmbedImage{URL: thumbnailURL},
				Description: fmt.Sprintf("%.300v...", np.Items[0].Snippet.Description),
				Author:      &discordgo.MessageEmbedAuthor{},
				Color:       rand.Intn(16777215), // rand
				Fields:      fields,
				URL:         fmt.Sprintf("http://youtu.be/%v", np.Items[0].Id),
				Timestamp:   time.Now().Format(time.RFC3339)}) // Discord wants ISO8601; RFC3339 is an extension of ISO8601 and should be completely compatible.
		}
	case "playtop", "pt", "playnext", "pn":
		commandPlay(discord, message, command, commandContents, true)
	case "ban":
		if message.Author.ID != botOwner || len(commandContents) != 2 {
			// return silently as to stay hidden
			// this doesnt ban permanently or
			// even ban from the server
			return
		}
		banList[message.GuildID] = append(banList[message.GuildID], commandContents[1])
		log.Trace("Banned '", commandContents[1], "'")
	case "unban":
		if message.Author.ID != botOwner || len(commandContents) != 2 {
			// return silently as to stay hidden
			// this doesnt ban permanently or
			// even ban from the server
			return
		}
		for j, i := range banList[message.GuildID] {
			if i == commandContents[1] {
				banList[message.GuildID] = removeFromSlice(banList[message.GuildID], j)
			}
		}
	case "pause":
		pausedMap[message.GuildID] = true
		stopMap[message.GuildID] <- true
	case "resume":
		stopMap[message.GuildID] <- false
	case "fix":
		if message.Author.ID != botOwner || len(commandContents) != 2 {
			// return silently as to stay hidden
			return
		}
		delete(ytdlCache, commandContents[1])
		discord.ChannelMessageSend(message.ChannelID, fmt.Sprintf(
			"Removed song with url %v", commandContents[0]))
	case "lyrics":
		getLyrics(discord, message, command, commandContents)
	}
}

//getLyrics get's the lyrics the user requested and prints them to the chat
//yes, it's a bit long. I know.
func getLyrics(discord *discordgo.Session, message *discordgo.MessageCreate,
	command string, commandContents []string) {
	var song string

	switch {
	case len(commandContents) >= 2:
		song = url.QueryEscape(strings.Join(commandContents[1:], " "))
	case queue[message.GuildID] == nil:
		discord.ChannelMessageSend(message.ChannelID, "No queue for this server")
	case len(queue[message.GuildID]) == 0:
		discord.ChannelMessageSend(message.ChannelID, "Nothing in the queue")
	default:
		np := ytdlCache[queue[message.GuildID][0]]

		//no one could ever play music from someone who isnt the artist
		channel := np.Items[0].Snippet.ChannelTitle
		for _, i := range []string{"VEVO", "- Topic", "(Video)", "(Official Video)", "M/V", "official",
			"explicit"} {
			channel = strings.TrimSuffix(channel, i)
		}

		title := np.Items[0].Snippet.Title
		if (strings.Contains(title, "-") || strings.Contains(title, "|")) && len(title)-2 >= len(channel) {

			switch {
			case strings.HasPrefix(strings.ToLower(title), strings.ToLower(channel)):
				title = title[len(channel)+2:]
			case strings.HasSuffix(strings.ToLower(title), strings.ToLower(channel)):
				title = title[:len(channel)-2]
			}
		}

		for _, i := range []string{"VEVO", "- Topic", "(Video)", "(Official Video)", "M/V", "(Explicit)",
			"(Clean)"} {
			title = strings.TrimSuffix(title, i)
		}

		title = strings.TrimSpace(title)
		channel = strings.TrimSpace(channel)

		song = url.QueryEscape(fmt.Sprintf("%v By %v", title, channel))
		log.Debugf("Lyrics for %v by %v", title, channel)
	}

	var l Lyrics
	//save count, if this goes over 10 then bail out and assume there was some other glitch or
	//that my validity tests failed
	var count uint8

lyricGettingLoop:
	for {
		count++
		// avoid indefinite loops from the api being bad at existing
		if count > 20 {
			discord.ChannelMessageSend(message.ChannelID,
				"There's some repeating error with the API, maybe try again or contact the bot owner")
			break lyricGettingLoop
		}

		qURL := fmt.Sprintf("https://some-random-api.ml/lyrics?title=%v",
			song)
		log.Tracef(qURL)
		resp, err := lyricHTTPClient.Get(qURL)
		if err != nil {
			//			discord.ChannelMessageSend(message.ChannelID, fmt.Sprintf(
			//				"Error getting lyrics, wont try from youtube because their API is garbage: %v",
			//				err))
			log.Errorln("Error getting lyrics: ", err)
			continue lyricGettingLoop

		}

		defer resp.Body.Close()

		decoder := json.NewDecoder(resp.Body)
		err = decoder.Decode(&l)
		if err != nil {
			//			discord.ChannelMessageSend(message.ChannelID, fmt.Sprintf(
			//				"Error unmarshalling lyrics, wont try from youtube because their API is garbage: %v",
			//				err))
			log.Errorln("Error unmarshalling lyrics: ", err)
			continue lyricGettingLoop
		}

		switch l.Error {
		case "Sorry I couldn't find that song's lyrics":
			discord.ChannelMessageSend(message.ChannelID,
				"The api says no song, so idk what to tell you")
			return
		case "Too many requests, please try again later.":
			time.Sleep(5*time.Second)
			continue lyricGettingLoop
		case "":
		default:
			log.Errorf("Error returned from API: %v", l.Error)
			continue
		}

		//check for newlines
		//there's a bug in this api where sometimes there just arent new lines
		//dont ask me why, it's bloody annoying
		//I think 20 characters for an intentionally one line song is fair, if this causes problems
		//then I just don't know
		switch {
		case l == Lyrics{} && count >= 20:
			discord.ChannelMessageSend(message.ChannelID,
				fmt.Sprintf(
					"We tried getting you lyrics %v times but ended up with no result, please check for typos",
					count))
			continue lyricGettingLoop
		case strings.Contains(l.Lyrics, "\n") || len(l.Lyrics) <= 20:
		case count >= 10:
			discord.ChannelMessageSend(message.ChannelID,
				fmt.Sprintf(
					"While getting lyrics, this failed %v times to get a response I consider valid, "+
						"this song may be formatted without line breaks, if so, please try the query again.",
					count))
		default:
			continue lyricGettingLoop
		}

		break lyricGettingLoop
	}

	//too verbose, disable normally
	//log.Traceln(l)

	//replace \n's with newlines
	l.Lyrics = fmt.Sprintln(l.Lyrics)

	color := rand.Intn(16777215)
	embed := genLyricsEmbed(l, color)

	//only do thumbnail for first embed
	embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: l.Thumbnail.Genius}

	if len(l.Lyrics) < 1000 {
		embed.Fields =
			[]*discordgo.MessageEmbedField{
				&discordgo.MessageEmbedField{
					Name:  "Lyrics",
					Value: l.Lyrics,
				}}
	} else {
		buf := bytes.NewReader([]byte(l.Lyrics))
		r := bufio.NewScanner(buf)
		r.Split(bufio.ScanLines)

		var txtlines []string

		for r.Scan() {
			txtlines = append(txtlines, r.Text())
		}

		discord.ChannelMessageSendEmbed(message.ChannelID, &discordgo.MessageEmbed{
			Title:  fmt.Sprintf("%v by %v", l.Title, l.Author),
			Author: &discordgo.MessageEmbedAuthor{},
			Color:  color,
			Fields: []*discordgo.MessageEmbedField{
				&discordgo.MessageEmbedField{
					Name: "Note",
					Value: `Lyrics too big, using multiple messages and/or fields. 
					Blame discord for this limitation`,
				}},
			Timestamp: time.Now().Format(time.RFC3339)})

		var msg string
		var fields []*discordgo.MessageEmbedField
		for _, eachline := range txtlines {
			msg += fmt.Sprintln(eachline)
			if len(msg) > 900 {

				fields = append(fields,
					&discordgo.MessageEmbedField{
						Name:  "Lyrics:",
						Value: msg,
					})

				if len(fields) == 5 {
					embed.Fields = fields
					fields = []*discordgo.MessageEmbedField{}
					_, err := discord.ChannelMessageSendEmbed(message.ChannelID, embed)
					if err != nil {
						log.Errorln("error sending partial lyrics embed", err)
					}

				}
				msg = ""
			}
		}
		if msg != "" {
			fields = append(fields,
				&discordgo.MessageEmbedField{
					Name:  "Lyrics:",
					Value: msg,
				})
			embed = genLyricsEmbed(l, color)
		}
		if len(fields) != 0 {
			embed.Fields = fields
		}
	}

	_, err := discord.ChannelMessageSendEmbed(message.ChannelID, embed)
	if err != nil {
		log.Errorln("error sending lyrics embed", err)
	}
}

func genLyricsEmbed(l Lyrics, color int) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:     fmt.Sprintf("%v by %v", l.Title, l.Author),
		Author:    &discordgo.MessageEmbedAuthor{},
		Color:     color,
		Timestamp: time.Now().Format(time.RFC3339),
		URL:       l.Links.Genius}
}

func unique(intSlice []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range intSlice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

func returnPlaylist(input string) ([]string, error) {

	ctx := context.Background()
	service, err := youtube.NewService(ctx, option.WithAPIKey(Config.googleDeveloperKey))
	if err != nil {
		return nil, err
	}

	// Make the API call to YouTube.
	call := service.Search.List("id").
		Q(input).
		MaxResults(1)
	response, err := call.Do()
	if err != nil {
		return nil, err
	}

	var result *youtube.SearchResult
	for _, i := range response.Items {
		if i.Id.Kind == "youtube#playlist" {
			// this is cheap enough for now to not bother with cache
			//youtubeSearchCache[input] = i
			result = i
		}
	}

	if result == nil {
		return nil, fmt.Errorf("no playlist in query")

	}

	itemCall := service.PlaylistItems.List("id,snippet").
		PlaylistId(result.Id.PlaylistId).
		MaxResults(50)

	playlistResp, err := itemCall.Do()
	if err != nil {
		return nil, err
	}

	var listOfVideos []string

	for _, i := range playlistResp.Items {
		out, _ := dlToTmp(i.Snippet.ResourceId.VideoId)

		// this is stupid and will max out my quota. Too bad!
		// This also doesnt work always, but I dont have the energy to make it better.
		sr, err := getVideoInfo(i.Snippet.ResourceId.VideoId, service)
		//fmt.Printf("%+v", i)
		if err != nil {
			log.Errorln(err)
			continue
		}

		ytdlCache[out] = sr
		listOfVideos = append(listOfVideos, out)
	}

	return listOfVideos, nil

}

// doesnt get suggestions because this joke of an API is too damn expensive
func getVideoInfo(result string, service *youtube.Service) (*youtube.VideoListResponse, error) {
	vidService := youtube.NewVideosService(service)

	return vidService.List("snippet,id").Id(result).Do()
}

func searchForVideo(input string) (*youtube.VideoListResponse, error) {

	// sloppy way of keeping my quota intact
	if val, ok := youtubeSearchCache[input]; ok {
		log.Traceln("Getting search from cache (yay!)")
		return val, nil
	}

	ctx := context.Background()
	service, err := youtube.NewService(ctx, option.WithAPIKey(Config.googleDeveloperKey))
	if err != nil {
		return nil, err
	}

	if youtubeURLRegex.MatchString(input) {
		log.Debugln("The input is a URL! Yay for cheap!")
		return getVideoInfo(youtubeURLRegex.FindAllStringSubmatch(input, 2)[0][1], service)
	}

	// Each one of these API quotas costs me 100 quota points
	// I shouldnt have to pay that much for a goddamn search
	// this will max out my quota, too bad!
	call := service.Search.List("id").
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
			// this is f-ing stupid, but I need it to process enough videos.
			vid, err := getVideoInfo(i.Id.VideoId, service)
			if err != nil {
				log.Errorln(err)
				continue
			}
			youtubeSearchCache[input] = vid
			return vid, nil
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
	youtubeDl.Options.Output.Value = filepath.Join(tmpdir, subdir, idSplit[0], idSplit[1], fmt.Sprintf("%v.%%(ext)s", url))
	youtubeDl.Options.ExtractAudio.Value = true
	youtubeDl.Options.AudioFormat.Value = "mp3"
	youtubeDl.Options.KeepVideo = goydl.BoolOption{Value: false} // why is this a thing
	//	youtubeDl.Options.AudioQuality = goydl.StringOption{Value: "0"}

	youtubeDl.VideoURL = fmt.Sprintf("www.youtube.com/watch?v=%v", url)
	// listen to errors from ydl
	//		go io.Copy(os.Stdout, youtubeDl.Stdout)
	//		go io.Copy(os.Stderr, youtubeDl.Stderr)

	log.Traceln(youtubeDl.Info)

	dwnld, err := youtubeDl.Download()
	//panic("testing download error")
	if err != nil {
		//		log.Debugf("Path: %v", dwnld.Path)
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

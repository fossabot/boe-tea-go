package commands

import (
	"errors"
	"fmt"
	"image"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/VTGare/boe-tea-go/database"
	"github.com/VTGare/boe-tea-go/images"
	"github.com/VTGare/boe-tea-go/services"
	"github.com/VTGare/boe-tea-go/utils"
	"github.com/bwmarrin/discordgo"
	log "github.com/sirupsen/logrus"
)

var (
	//ImageURLRegex is a regex for image URLs
	ImageURLRegex = regexp.MustCompile(`(?i)(http(s?):)([/|.|\w|\s|-])*\.(?:jpg|jpeg|gif|png|webp)`)
	ErrNoSauce    = errors.New("source image has not been found")
	searchEngines = map[string]func(link string) (*discordgo.MessageEmbed, error){
		"saucenao": saucenao,
		"ascii2d":  ascii2d,
	}
)

func init() {
	Commands["sauce"] = Command{
		Name:            "sauce",
		Description:     "Finds sauce of an anime picture on SauceNAO or ascii2d.",
		GuildOnly:       false,
		Exec:            sauce,
		Help:            true,
		AdvancedCommand: true,
		ExtendedHelp: []*discordgo.MessageEmbedField{
			{
				Name:  "Usage",
				Value: "bt!sauce <search engine> <image link>",
			},
			{
				Name:  "Reverse image search engine",
				Value: "Not required. ``saucenao`` or ``ascii2d``. If omitted uses server's default option",
			},
			{
				Name:  "image link",
				Value: "Required. Link must have one of the following suffixes:  *jpg*, *jpeg*, *png*, *gif*, *webp*.\nURL parameters after the link are acceptable (e.g. <link>.jpg***?width=441&height=441***)",
			},
		},
	}
	Commands["pixiv"] = Command{
		Name:            "pixiv",
		Description:     "Reposts a single Pixiv post",
		GuildOnly:       false,
		Exec:            pixiv,
		Help:            true,
		AdvancedCommand: true,
		ExtendedHelp: []*discordgo.MessageEmbedField{
			{
				Name:  "Usage",
				Value: "bt!pixiv <post link> [optional excluded images]",
			},
			{
				Name:  "excluded images",
				Value: "Indexes must be separated by whitespace (e.g. 1 2 4 6 10 45)",
			},
		},
	}
	Commands["deepfry"] = Command{
		Name:            "deepfry",
		Description:     "Deepfries an image, cursed as hell",
		GuildOnly:       false,
		Exec:            deepfry,
		Help:            true,
		AdvancedCommand: true,
		ExtendedHelp: []*discordgo.MessageEmbedField{
			{
				Name:  "Usage",
				Value: "bt!deepfry <optional times deepfried> <image link>",
			},
			{
				Name:  "times deepfried",
				Value: "Repeats deepfrying process given amount of times, up to 5.",
			},
			{
				Name:  "image link",
				Value: "Image link, if not present uses an attachment.",
			},
		},
	}
	Commands["twitter"] = Command{
		Name:            "twitter",
		Description:     "Reposts a twitter post",
		GuildOnly:       false,
		Exec:            twitter,
		Help:            true,
		AdvancedCommand: true,
		ExtendedHelp: []*discordgo.MessageEmbedField{
			{
				Name:  "Usage",
				Value: "bt!twitter <twitter link>",
			},
			{
				Name:  "Twitter link",
				Value: "Must look something like this: https://twitter.com/mhy_shima/status/1258684420011069442",
			},
		},
	}
}

func sauce(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	if len(m.Attachments) > 0 {
		args = append(args, m.Attachments[0].URL)
	}

	messages, err := s.ChannelMessages(m.ChannelID, 5, m.ID, "", "")
	if err != nil {
		return err
	}

	if recent := findRecentImage(messages); recent != "" {
		args = append(args, recent)
	}

	findEngine := func() string {
		if m.GuildID != "" {
			return database.GuildCache[m.GuildID].ReverseSearch
		}
		return "saucenao"
	}

	url := ""
	searchEngine := ""

	if len(args) == 0 {
		return utils.ErrNotEnoughArguments
	} else if len(args) == 1 {
		searchEngine = findEngine()
		url = ImageURLRegex.FindString(args[0])
		if url == "" {
			return errors.New("received a non-image url")
		}
	} else if len(args) >= 2 {
		if f := ImageURLRegex.FindString(args[0]); f != "" {
			searchEngine = findEngine()
			url = f
		} else if _, ok := searchEngines[args[0]]; ok {
			searchEngine = args[0]
			url = ImageURLRegex.FindString(args[1])
		} else {
			return errors.New("incorrect command usage, please use bt!help sauce for more info")
		}

		if url == "" {
			return errors.New("received a non-image url")
		}
	}

	log.Infoln("Searching sauce for", url, "on", searchEngine)
	embed, err := searchEngines[searchEngine](url)
	if err != nil {
		return err
	}
	_, err = s.ChannelMessageSendEmbed(m.ChannelID, embed)
	if err != nil {
		return err
	}

	return nil
}

func ascii2d(link string) (*discordgo.MessageEmbed, error) {
	results, err := services.GetSauceA2D(link)
	if err != nil {
		return nil, err
	}

	for _, res := range results {
		if res.Author == "" || res.AuthorURL == "" || res.From == "" || res.Name == "" || res.Thumbnail == "" || res.URL == "" {
			continue
		}

		log.Infoln("source found", res)
		embed := &discordgo.MessageEmbed{
			Title: fmt.Sprintf("%v by %v on %v", res.Name, res.Author, res.From),
			URL:   res.URL,
			Thumbnail: &discordgo.MessageEmbedThumbnail{
				URL: res.Thumbnail,
			},
			Color:     utils.EmbedColor,
			Timestamp: utils.EmbedTimestamp(),
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:  "Name",
					Value: res.Name,
				},
				{
					Name:  "URL",
					Value: res.URL,
				},
				{
					Name:  "Author",
					Value: res.Author,
				},
				{
					Name:  "Author URL",
					Value: res.AuthorURL,
				},
			},
		}
		return embed, nil
	}
	return nil, ErrNoSauce
}

func saucenao(link string) (*discordgo.MessageEmbed, error) {
	saucenao, err := services.SearchSauceByURL(link)
	if err != nil {
		return nil, err
	}
	results, err := utils.FilterLowSimilarity(saucenao.Results)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, ErrNoSauce
	}

	findSauce := func() *services.Sauce {
		for _, res := range results {
			if len(res.Data.URLs) == 0 {
				continue
			}

			if res.Data.Title == "" {
				res.Data.Title = "Sauce"
			}

			return res
		}
		return nil
	}

	res := findSauce()
	if res == nil {
		return nil, ErrNoSauce
	}

	author := utils.FindAuthor(*res)
	log.Infoln("Source found. URL: %v. Title: %v", res.Data.URLs[0], res.Data.Title)
	embed := &discordgo.MessageEmbed{
		Title:     res.Data.Title,
		URL:       res.Data.URLs[0],
		Timestamp: utils.EmbedTimestamp(),
		Color:     utils.EmbedColor,
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: res.Header.Thumbnail,
		},
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  "URL",
				Value: res.Data.URLs[0],
			},
			{
				Name:  "Similarity",
				Value: res.Header.Similarity,
			},
			{
				Name:  "Author",
				Value: author,
			},
		},
	}
	return embed, nil
}

func findRecentImage(messages []*discordgo.Message) string {
	for _, msg := range messages {
		f := ImageURLRegex.FindString(msg.Content)
		switch {
		case f != "":
			return f
		case len(msg.Attachments) > 0:
			return msg.Attachments[0].URL
		case len(msg.Embeds) > 0:
			if msg.Embeds[0].Image != nil {
				return msg.Embeds[0].Image.URL
			}
		}
	}

	return ""
}

func pixiv(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	if len(args) == 0 {
		return utils.ErrNotEnoughArguments
	}

	match := utils.PixivRegex.FindStringSubmatch(args[0])
	if match == nil {
		return errors.New("first arguments must be a pixiv link")
	}

	args = args[1:]
	excludes := make(map[int]bool)
	for _, arg := range args {
		if strings.Contains(arg, "-") {
			ran, err := utils.NewRange(arg)
			if err != nil {
				return err
			}

			for i := ran.Low; i <= ran.High; i++ {
				excludes[i] = true
			}
		} else {
			num, err := strconv.Atoi(arg)
			if err != nil {
				return utils.ErrParsingArgument
			}
			excludes[num] = true
		}
	}

	utils.PostPixiv(s, m, []string{match[1]}, utils.PixivOptions{
		ProcPrompt: false,
		Indexes:    excludes,
		Exclude:    true,
	})
	return nil
}

func twitter(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	if len(args) == 0 {
		return utils.ErrNotEnoughArguments
	}

	tweet, err := services.GetTweet(args[0])
	if err != nil {
		return err
	}

	messages := make([]discordgo.MessageSend, 0)
	for ind, media := range tweet.Gallery {
		title := ""
		if len(tweet.Gallery) > 1 {
			title = fmt.Sprintf("%v's tweet | Page %v/%v", tweet.Author, ind+1, len(tweet.Gallery))
		} else {
			title = fmt.Sprintf("%v's tweet", tweet.Author)
		}

		embed := discordgo.MessageEmbed{
			Title:     title,
			Timestamp: tweet.Timestamp,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "Likes",
					Value:  strconv.Itoa(tweet.Likes),
					Inline: true,
				},
				{
					Name:   "Retweets",
					Value:  strconv.Itoa(tweet.Retweets),
					Inline: true,
				},
			},
		}

		msg := discordgo.MessageSend{}
		if ind == 0 {
			embed.Description = tweet.Content
		}

		if media.Animated {
			resp, err := http.Get(media.URL)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			filename := media.URL[strings.LastIndex(media.URL, "/")+1:]
			msg.File = &discordgo.File{
				Name:   filename,
				Reader: resp.Body,
			}
		} else {
			embed.Image = &discordgo.MessageEmbedImage{
				URL: media.URL,
			}
		}
		msg.Embed = &embed

		messages = append(messages, msg)
	}

	for _, message := range messages {
		_, err := s.ChannelMessageSendComplex(m.ChannelID, &message)
		if err != nil {
			return err
		}
	}
	return nil
}

func deepfry(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	if len(m.Attachments) > 0 {
		args = append(args, m.Attachments[0].URL)
	}

	url := ""
	times := 0
	switch len(args) {
	case 2:
		if f := ImageURLRegex.FindString(args[0]); f != "" {
			url = f
		} else {
			var err error
			times, err = strconv.Atoi(args[0])
			if times > 5 {
				return errors.New("can't deepfry more than 5 times at once")
			}
			if err != nil {
				return err
			}
			url = ImageURLRegex.FindString(args[1])
		}

		if url == "" {
			return errors.New("received a non-image url")
		}
	case 1:
		if f := ImageURLRegex.FindString(args[0]); f != "" {
			url = f
		} else {
			return errors.New("received a non-image url")
		}
	case 0:
		return utils.ErrNotEnoughArguments
	}

	img, err := images.DownloadImage(url)
	if err != nil {
		return err
	}

	deepfried := images.Deepfry(img)
	for i := 0; i < times; i++ {
		img, _, _ := image.Decode(deepfried)
		deepfried = images.Deepfry(img)
	}

	s.ChannelFileSend(m.ChannelID, "deepfried.jpg", deepfried)
	return nil
}

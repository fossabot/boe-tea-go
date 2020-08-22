package bot

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ReneKroon/ttlcache"
	"github.com/VTGare/boe-tea-go/internal/commands"
	"github.com/VTGare/boe-tea-go/internal/database"
	"github.com/VTGare/boe-tea-go/internal/repost"
	"github.com/VTGare/boe-tea-go/utils"
	"github.com/bwmarrin/discordgo"
	log "github.com/sirupsen/logrus"
)

var (
	botMention   string
	messageCache *ttlcache.Cache
)

type Bot struct {
	s *discordgo.Session
}

func (b *Bot) Run() error {
	if err := b.s.Open(); err != nil {
		return err
	}

	defer b.s.Close()
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, syscall.SIGSEGV, syscall.SIGHUP)
	<-sc

	return nil
}

func NewBot(token string) (*Bot, error) {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}

	bot := &Bot{dg}
	dg.AddHandler(bot.messageCreated)
	dg.AddHandler(bot.onReady)
	dg.AddHandler(bot.reactCreated)
	dg.AddHandler(bot.messageDeleted)
	dg.AddHandler(bot.guildCreated)
	dg.AddHandler(bot.guildDeleted)
	return bot, nil
}

type cachedMessage struct {
	Parent   *discordgo.Message
	Children []*discordgo.Message
}

func init() {
	messageCache = ttlcache.NewCache()
	messageCache.SetTTL(15 * time.Minute)
}

func (b *Bot) onReady(s *discordgo.Session, e *discordgo.Ready) {
	botMention = "<@!" + e.User.ID + ">"
	log.Infoln(e.User.String(), "is ready.")

	err := utils.CreateDB(e.Guilds)
	if err != nil {
		log.Warnln("Error adding guilds: ", err)
	}
}

func handleError(s *discordgo.Session, m *discordgo.MessageCreate, err error) {
	if err != nil {
		log.Errorf("An error occured: %v", err)
		embed := &discordgo.MessageEmbed{
			Title: "Oops, something went wrong!",
			Thumbnail: &discordgo.MessageEmbedThumbnail{
				URL: utils.DefaultEmbedImage,
			},
			Description: fmt.Sprintf("***Error message:***\n%v\n\nPlease contact bot's author using bt!feedback command or directly at VTGare#3370 if you can't understand the error.", err),
			Color:       utils.EmbedColor,
			Timestamp:   utils.EmbedTimestamp(),
		}
		s.ChannelMessageSendEmbed(m.ChannelID, embed)
	}
}

func (b *Bot) prefixless(s *discordgo.Session, m *discordgo.MessageCreate) error {
	art := repost.NewPost(*m)
	guild := database.GuildCache[m.GuildID]

	if guild.Repost != "disabled" {
		art.FindReposts()
		if len(art.Reposts) > 0 {
			if guild.Repost == "strict" {
				art.RemoveReposts()
				s.ChannelMessageSendEmbed(m.ChannelID, art.RepostEmbed())
				if art.Len() == 0 {

					s.ChannelMessageDelete(m.ChannelID, m.ID)
				}
			} else if guild.Repost == "enabled" {
				if art.PixivReposts() > 0 && guild.Pixiv {
					prompt := utils.CreatePromptWithMessage(s, m, &discordgo.MessageSend{
						Content: "Following posts are reposts, react 👌 to post them.",
						Embed:   art.RepostEmbed(),
					})
					if !prompt {
						return nil
					}
				} else {
					s.ChannelMessageSendEmbed(m.ChannelID, art.RepostEmbed())
				}
			}
		}
	}

	if guild.Pixiv {
		messages, err := art.SendPixiv(s)
		if err != nil {
			return err
		}

		embeds := make([]*discordgo.Message, 0)
		keys := make([]string, 0)
		keys = append(keys, m.Message.ID)

		for _, message := range messages {
			embed, _ := s.ChannelMessageSendComplex(m.ChannelID, message)

			if embed != nil {
				keys = append(keys, embed.ID)
				embeds = append(embeds, embed)
			}
		}

		if art.HasUgoira {
			art.Cleanup()
		}

		c := &cachedMessage{m.Message, embeds}
		for _, key := range keys {
			messageCache.Set(key, c)
		}
	}

	if guild.Twitter && len(art.TwitterMatches) > 0 {
		tweets, err := art.SendTwitter(s, true)
		if err != nil {
			return err
		}

		if len(tweets) > 0 {
			msg := ""
			if len(tweets) == 1 {
				msg = "Detected a tweet with more than one image, would you like to send embeds of other images for mobile users?"
			} else {
				msg = "Detected tweets with more than one image, would you like to send embeds of other images for mobile users?"
			}

			prompt := utils.CreatePrompt(s, m, &utils.PromptOptions{
				Actions: map[string]bool{
					"👌": true,
				},
				Message: msg,
				Timeout: 10 * time.Second,
			})

			if prompt {
				for _, t := range tweets {
					for _, send := range t {
						_, err := s.ChannelMessageSendComplex(m.ChannelID, send)
						if err != nil {
							log.Warnln(err)
						}
					}
				}
			}
		}
	}

	return nil
}

func (b *Bot) messageCreated(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.Bot {
		return
	}

	isCommand := commands.CommandFramework.Handle(s, m)
	if !isCommand {
		err := b.prefixless(s, m)
		commands.CommandFramework.ErrorHandler(err)
	}
}

func (b *Bot) reactCreated(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	if messageCache.Count() > 0 && r.Emoji.APIName() == "❌" {
		if m, ok := messageCache.Get(r.MessageID); ok {
			c := m.(*cachedMessage)
			if r.UserID == c.Parent.Author.ID {
				if r.MessageID == c.Parent.ID {
					s.ChannelMessageDelete(c.Parent.ChannelID, c.Parent.ID)
					messageCache.Remove(c.Parent.ID)
					for _, child := range c.Children {
						s.ChannelMessageDelete(child.ChannelID, child.ID)
						messageCache.Remove(child.ID)
					}
				} else {
					s.ChannelMessageDelete(r.ChannelID, r.MessageID)
					messageCache.Remove(r.MessageID)
				}
			}
		}
	}
}

func (b *Bot) messageDeleted(s *discordgo.Session, m *discordgo.MessageDelete) {
	if messageCache.Count() > 0 {
		if mes, ok := messageCache.Get(m.ID); ok {
			c := mes.(*cachedMessage)
			if c.Parent.ID == m.ID {
				s.ChannelMessageDelete(c.Parent.ChannelID, c.Parent.ID)
				messageCache.Remove(c.Parent.ID)
				for _, child := range c.Children {
					s.ChannelMessageDelete(child.ChannelID, child.ID)
					messageCache.Remove(child.ID)
				}
			} else {
				for ind, child := range c.Children {
					if child.ID == m.ID {
						messageCache.Remove(child.ID)
						c.Children = append(c.Children[:ind], c.Children[ind+1:]...)
						break
					}
				}
			}
		}
	}
}

func (b *Bot) guildCreated(s *discordgo.Session, g *discordgo.GuildCreate) {
	if len(database.GuildCache) == 0 {
		return
	}

	if _, ok := database.GuildCache[g.ID]; !ok {
		newGuild := database.DefaultGuildSettings(g.ID)
		err := database.DB.InsertOneGuild(newGuild)
		if err != nil {
			log.Println(err)
		}

		database.GuildCache[g.ID] = newGuild
		log.Infoln("Joined ", g.Name)
	}
}

func (b *Bot) guildDeleted(s *discordgo.Session, g *discordgo.GuildDelete) {
	err := database.DB.RemoveGuild(g.ID)
	if err != nil {
		log.Println(err)
	}

	delete(database.GuildCache, g.ID)
	log.Infoln("Kicked or banned from", g.Guild.Name, g.ID)
}
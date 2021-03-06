package database

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	//GuildCache stores guild settings locally
	GuildCache = make(map[string]*GuildSettings)
)

//GuildSettings is a database model for per guild bot settings
type GuildSettings struct {
	ID            string    `bson:"guild_id" json:"guild_id"`
	Prefix        string    `bson:"prefix" json:"prefix"`
	Limit         int       `bson:"limit" json:"limit"`
	Pixiv         bool      `bson:"pixiv" json:"pixiv"`
	Twitter       bool      `bson:"twitter" json:"twitter"`
	TwitterPrompt bool      `bson:"twitterprompt" json:"twitterprompt"`
	Crosspost     bool      `bson:"crosspost" json:"crosspost"`
	NSFW          bool      `bson:"nsfw" json:"nsfw"`
	Repost        string    `bson:"repost" json:"repost"`
	CreatedAt     time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt     time.Time `bson:"updated_at" json:"updated_at"`
}

//DefaultGuildSettings returns a default GuildSettings struct.
func DefaultGuildSettings(guildID string) *GuildSettings {
	return &GuildSettings{
		ID:            guildID,
		Prefix:        "bt!",
		Limit:         10,
		Pixiv:         true,
		Twitter:       false,
		TwitterPrompt: false,
		Crosspost:     true,
		NSFW:          true,
		Repost:        "disabled",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

//AllGuilds returns all guilds from a database.
func (d *Database) AllGuilds() ([]*GuildSettings, error) {
	cur, err := d.GuildSettings.Find(context.Background(), bson.M{})

	if err != nil {
		return nil, err
	}

	guilds := make([]*GuildSettings, 0)
	cur.All(context.Background(), &guilds)

	if err != nil {
		return nil, fmt.Errorf("AllGuild(): %v", err)
	}

	for _, guild := range guilds {
		GuildCache[guild.ID] = guild
	}

	return guilds, nil
}

//InsertOneGuild inserts one guild to a database
func (d *Database) InsertOneGuild(guild *GuildSettings) error {
	_, err := d.GuildSettings.InsertOne(context.Background(), guild)
	if err != nil {
		return err
	}
	return nil
}

//InsertManyGuilds insert a bulk of guilds to a database
func (d *Database) InsertManyGuilds(guilds []interface{}) error {
	_, err := d.GuildSettings.InsertMany(context.Background(), guilds)
	if err != nil {
		return err
	}
	return nil
}

//RemoveGuild removes a guild from a database.
func (d *Database) RemoveGuild(guildID string) error {
	_, err := d.GuildSettings.DeleteOne(context.Background(), bson.M{"guild_id": guildID})
	if err != nil {
		return err
	}

	return nil
}

func (d *Database) ChangeSetting(guildID, setting string, newSetting interface{}) error {
	res := d.GuildSettings.FindOneAndUpdate(context.Background(), bson.M{
		"guild_id": guildID,
	}, bson.M{
		"$set": bson.M{
			setting:      newSetting,
			"updated_at": time.Now(),
		},
	}, options.FindOneAndUpdate().SetReturnDocument(options.After))

	guild := &GuildSettings{}
	err := res.Decode(guild)
	if err != nil {
		return err
	}

	GuildCache[guildID] = guild
	return nil
}

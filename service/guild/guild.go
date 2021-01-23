package main

import (
	"fmt"
	"os"

	config "github.com/JustHumanz/Go-Simp/pkg/config"
	database "github.com/JustHumanz/Go-Simp/pkg/database"
	"github.com/JustHumanz/Go-Simp/service/backend/utility/runfunc"
	"github.com/bwmarrin/discordgo"
	log "github.com/sirupsen/logrus"
)

var (
	H3llcome  = []string{config.Bonjour, config.Howdy, config.Guten, config.Koni, config.Selamat, config.Assalamu, config.Approaching}
	GuildList []string
	BotID     *discordgo.User
)

func main() {
	conf, err := config.ReadConfig("../../config.toml")
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	Bot, err := discordgo.New("Bot " + config.BotConf.Discord)
	if err != nil {
		log.Error(err)
	}
	err = Bot.Open()
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
	BotID, err = Bot.User("@me")
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	database.Start(conf.CheckSQL())

	for _, GuildID := range Bot.State.Guilds {
		GuildList = append(GuildList, GuildID.ID)
	}

	Bot.AddHandler(GuildJoin)
	log.Info("Guild handler ready.......")

	runfunc.Run(Bot)
}
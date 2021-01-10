package live

import (
	"regexp"
	"sync"
	"time"

	config "github.com/JustHumanz/Go-simp/tools/config"
	database "github.com/JustHumanz/Go-simp/tools/database"
	engine "github.com/JustHumanz/Go-simp/tools/engine"
	"github.com/bwmarrin/discordgo"
	log "github.com/sirupsen/logrus"
	"gopkg.in/robfig/cron.v2"
)

var (
	loc *time.Location
	Bot *discordgo.Session
)

//Start start twitter module
func Start(BotInit *discordgo.Session, cronInit *cron.Cron) {
	loc, _ = time.LoadLocation("Asia/Shanghai") /*Use CST*/
	Bot = BotInit
	cronInit.AddFunc(config.BiliBiliLive, CheckLiveSchedule)
	log.Info("Enable live bilibili module")
}

func CheckLiveSchedule() {
	loc, _ = time.LoadLocation("Asia/Shanghai")
	log.Info("Start check Schedule")
	for _, GroupData := range engine.GroupData {
		var wg sync.WaitGroup
		for i, MemberData := range database.GetMembers(GroupData.ID) {
			go CheckBili(GroupData, MemberData, &wg)
			if i%10 == 0 {
				wg.Wait()
			}
		}
	}
}

func CheckBili(Group database.Group, Member database.Member, wg *sync.WaitGroup) {
	defer wg.Done()
	if Member.BiliBiliID != 0 {
		var (
			ScheduledStart time.Time
			Data           LiveBili
		)
		DataDB, err := database.GetRoomData(Member.ID, Member.BiliRoomID)
		if err != nil {
			log.Error(err)
		}
		Status, err := GetRoomStatus(Member.BiliRoomID)
		if err != nil {
			log.Error(err)
			return
		}

		Data.AddData(DataDB)
		if Status.CheckScheduleLive() && DataDB.Status != "Live" {
			//Live
			if Status.Data.RoomInfo.LiveStartTime != 0 {
				ScheduledStart = time.Unix(int64(Status.Data.RoomInfo.LiveStartTime), 0).In(loc)
			} else {
				ScheduledStart = time.Now().In(loc)
			}

			GroupIcon := ""
			if match, _ := regexp.MatchString("404.jpg", Group.IconURL); match {
				GroupIcon = ""
			} else {
				GroupIcon = Group.IconURL
			}
			log.WithFields(log.Fields{
				"Group":  Group.GroupName,
				"Vtuber": Member.EnName,
				"Start":  ScheduledStart,
			}).Info("Start live right now")

			Data.SetStatus("Live").
				UpdateSchdule(ScheduledStart).
				UpdateOnline(Status.Data.RoomInfo.Online).
				SetMember(Member)

			err = Data.Crotttt(GroupIcon)
			if err != nil {
				log.Error(err)
			}

			Data.RoomData.UpdateLiveBili(Member.ID)

		} else if !Status.CheckScheduleLive() && DataDB.Status == "Live" {
			//prob past
			log.WithFields(log.Fields{
				"Group":  Group.GroupName,
				"Vtuber": Member.EnName,
				"Start":  ScheduledStart,
			}).Info("Past live stream")
			Data.SetStatus("Past").
				UpdateOnline(Status.Data.RoomInfo.Online)

			Data.RoomData.UpdateLiveBili(Member.ID)
		} else {
			//update online
			log.WithFields(log.Fields{
				"Group":  Group.GroupName,
				"Vtuber": Member.EnName,
			}).Info("Update LiveBiliBili")

			Data.UpdateOnline(Status.Data.RoomInfo.Online)
			Data.RoomData.UpdateLiveBili(Member.ID)
		}
	}
}
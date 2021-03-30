package youtube

import (
	"encoding/json"
	"encoding/xml"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hako/durafmt"

	"github.com/JustHumanz/Go-Simp/pkg/config"
	database "github.com/JustHumanz/Go-Simp/pkg/database"
	engine "github.com/JustHumanz/Go-Simp/pkg/engine"
	network "github.com/JustHumanz/Go-Simp/pkg/network"
	"github.com/JustHumanz/Go-Simp/service/livestream/bilibili/live"
	"github.com/JustHumanz/Go-Simp/service/livestream/notif"

	log "github.com/sirupsen/logrus"
)

//GetRSS GetRSS from Channel
func GetRSS(YtID string) []string {
	var (
		DataXML YtXML
		VideoID []string
	)

	Data, err := network.Curl("https://www.youtube.com/feeds/videos.xml?channel_id="+YtID+"&q=searchterms", nil)
	if err != nil {
		log.Error(err, string(Data))
	}

	xml.Unmarshal(Data, &DataXML)

	for i := 0; i < len(DataXML.Entry); i++ {
		VideoID = append(VideoID, DataXML.Entry[i].VideoId)
		if i == configfile.LimitConf.YoutubeLimit {
			break
		}
	}
	return VideoID
}

//StartCheckYT Youtube rss and API
func StartCheckYT(Group database.Group, wg *sync.WaitGroup) {
	defer wg.Done()
	for _, Member := range Group.Members {
		if Member.YoutubeID != "" {
			VideoID := GetRSS(Member.YoutubeID)
			Data, err := YtAPI(VideoID)
			if err != nil {
				log.Error(err)
			}

			log.WithFields(log.Fields{
				"Group":  Group.GroupName,
				"Member": Member.Name,
			}).Info("Checking Youtube Channels")

			for i, Items := range Data.Items {
				var (
					Viewers   string
					Thumb     string
					YtVideoID = VideoID[i]
				)

				YoutubeData, err := Member.CheckYoutubeVideo(YtVideoID)
				if err != nil {
					log.Error(err)
				}

				if Items.Snippet.VideoStatus == config.UpcomingStatus {
					if YoutubeData == nil {
						Viewers, err = GetWaiting(YtVideoID)
						if err != nil {
							log.Error(err)
						}
					} else if YoutubeData.Viewers != config.Ytwaiting {
						Viewers = YoutubeData.Viewers
					} else {
						Viewers, err = GetWaiting(YtVideoID)
						if err != nil {
							log.Error(err)
						}
					}
				} else if Items.LiveDetails.Viewers == "" {
					Viewers = Items.Statistics.ViewCount
				} else {
					Viewers = Items.LiveDetails.Viewers
				}

				if YoutubeData != nil {
					YoutubeData.
						UpdateViewers(Viewers).
						UpdateEnd(Items.LiveDetails.EndTime).
						UpdateLength(durafmt.Parse(ParseDuration(Items.ContentDetails.Duration)).String()).
						SetState(config.YoutubeLive).
						AddMember(Member).
						AddGroup(Group)

					if Items.Snippet.VideoStatus == "none" && YoutubeData.Status == config.LiveStatus {
						log.WithFields(log.Fields{
							"VideoData ID": YtVideoID,
							"Status":       config.PastStatus,
						}).Info("Update video status from " + Items.Snippet.VideoStatus + " to past")
						YoutubeData.UpdateYt(config.PastStatus)

						engine.RemoveEmbed(YtVideoID, Bot)

					} else if Items.Snippet.VideoStatus == config.LiveStatus && YoutubeData.Status == config.UpcomingStatus {
						log.WithFields(log.Fields{
							"VideoData ID": YtVideoID,
							"Status":       config.LiveStatus,
						}).Info("Update video status from " + YoutubeData.Status + " to live")
						YoutubeData.UpdateStatus(config.LiveStatus)

						log.Info("Send to notify")
						if !Items.LiveDetails.ActualStartTime.IsZero() {
							YoutubeData.UpdateSchdule(Items.LiveDetails.ActualStartTime)
						} else {
							notif.SendDude(YoutubeData, Bot)
						}

						YoutubeData.UpdateYt(YoutubeData.Status)

					} else if (!Items.LiveDetails.EndTime.IsZero() && YoutubeData.Status == config.UpcomingStatus) || (YoutubeData.Status == config.UpcomingStatus && Items.Snippet.VideoStatus == "none") {
						log.WithFields(log.Fields{
							"VideoData ID": YtVideoID,
							"Status":       config.PastStatus,
						}).Info("Update video status from " + Items.Snippet.VideoStatus + " to past,probably member only")
						YoutubeData.UpdateYt(config.PastStatus)

						engine.RemoveEmbed(YtVideoID, Bot)

					} else if Items.Snippet.VideoStatus == config.UpcomingStatus && YoutubeData.Status == config.PastStatus {
						log.WithFields(log.Fields{
							"VideoData ID": YtVideoID,
							"Status":       Items.Snippet.VideoStatus,
						}).Info("maybe yt error or human error")

						YoutubeData.UpdateStatus(config.UpcomingStatus)
						notif.SendDude(YoutubeData, Bot)

					} else if Items.Snippet.VideoStatus == "none" && YoutubeData.Viewers != Items.Statistics.ViewCount {
						log.WithFields(log.Fields{
							"VideoData ID": YtVideoID,
							"Viwers past":  YoutubeData.Viewers,
							"Viwers now":   Items.Statistics.ViewCount,
							"Status":       config.PastStatus,
						}).Info("Update Viwers")
						YoutubeData.UpdateYt(config.PastStatus)

					} else if Items.Snippet.VideoStatus == config.LiveStatus {
						log.WithFields(log.Fields{
							"VideoData id": YtVideoID,
							"Viwers Live":  Items.Statistics.ViewCount,
							"Status":       config.LiveStatus,
						}).Info("Update Viwers")
						YoutubeData.UpdateYt(config.LiveStatus)

					} else if Items.Snippet.VideoStatus == config.UpcomingStatus {
						if Items.LiveDetails.StartTime != YoutubeData.Schedul {
							log.WithFields(log.Fields{
								"VideoData ID": YtVideoID,
								"old schdule":  YoutubeData.Schedul,
								"new schdule":  Items.LiveDetails.StartTime,
								"Status":       config.UpcomingStatus,
							}).Info("Livestream schdule changed")

							YoutubeData.UpdateSchdule(Items.LiveDetails.StartTime)
							YoutubeData.UpdateYt(config.UpcomingStatus)
						}
					} else {
						YoutubeData.UpdateYt(YoutubeData.Status)
					}
				} else {
					_, err := network.Curl("http://i3.ytimg.com/vi/"+YtVideoID+"/maxresdefault.jpg", nil)
					if err != nil {
						Thumb = "http://i3.ytimg.com/vi/" + YtVideoID + "/hqdefault.jpg"
					} else {
						Thumb = "http://i3.ytimg.com/vi/" + YtVideoID + "/maxresdefault.jpg"
					}

					YtType := engine.YtFindType(Items.Snippet.Title)
					if YtType == "Streaming" && Items.ContentDetails.Duration != "P0D" && Items.LiveDetails.StartTime.IsZero() {
						YtType = "Regular video"
					}

					NewYoutubeData := &database.LiveStream{
						Status:    Items.Snippet.VideoStatus,
						VideoID:   YtVideoID,
						Title:     Items.Snippet.Title,
						Thumb:     Thumb,
						Desc:      Items.Snippet.Description,
						Schedul:   Items.LiveDetails.StartTime,
						Published: Items.Snippet.PublishedAt,
						Type:      YtType,
						Viewers:   Viewers,
						Length:    durafmt.Parse(ParseDuration(Items.ContentDetails.Duration)).String(),
						Member:    Member,
						Group:     Group,
						State:     config.YoutubeLive,
					}

					if Items.Snippet.VideoStatus == config.UpcomingStatus {
						log.WithFields(log.Fields{
							"YtID":       YtVideoID,
							"MemberName": Member.EnName,
							"Message":    "Send to notify",
						}).Info("New Upcoming live schedule")

						NewYoutubeData.UpdateStatus(config.UpcomingStatus)
						_, err := NewYoutubeData.InputYt()
						if err != nil {
							log.Error(err)
						}
						notif.SendDude(NewYoutubeData, Bot)

					} else if Items.Snippet.VideoStatus == config.LiveStatus {
						log.WithFields(log.Fields{
							"YtID":       YtVideoID,
							"MemberName": Member.EnName,
							"Message":    "Send to notify",
						}).Info("New live stream right now")

						NewYoutubeData.UpdateStatus(config.LiveStatus)
						_, err := NewYoutubeData.InputYt()
						if err != nil {
							log.Error(err)
						}

						if Member.BiliRoomID != 0 {
							LiveBili, err := live.GetRoomStatus(Member.BiliRoomID)
							if err != nil {
								log.Error(err)
							}
							if LiveBili.CheckScheduleLive() {
								NewYoutubeData.SetBiliLive(true).UpdateBiliToLive()
							}
						}

						if !Items.LiveDetails.ActualStartTime.IsZero() {
							NewYoutubeData.UpdateSchdule(Items.LiveDetails.ActualStartTime)
							notif.SendDude(NewYoutubeData, Bot)

						} else {
							notif.SendDude(NewYoutubeData, Bot)
						}

					} else if Items.Snippet.VideoStatus == "none" && YtType == "Covering" {
						log.WithFields(log.Fields{
							"YtID":       YtVideoID,
							"MemberName": Member.EnName,
						}).Info("New MV or Cover")

						NewYoutubeData.UpdateStatus(config.PastStatus).InputYt()
						notif.SendDude(NewYoutubeData, Bot)

					} else if !Items.Snippet.PublishedAt.IsZero() && Items.Snippet.VideoStatus == "none" {
						log.WithFields(log.Fields{
							"YtID":       YtVideoID,
							"MemberName": Member.EnName,
						}).Info("Suddenly upload new video")
						if NewYoutubeData.Schedul.IsZero() {
							NewYoutubeData.UpdateSchdule(NewYoutubeData.Published)
						}

						NewYoutubeData.UpdateStatus(config.PastStatus).InputYt()
						notif.SendDude(NewYoutubeData, Bot)

					} else {
						log.WithFields(log.Fields{
							"YtID":       YtVideoID,
							"MemberName": Member.EnName,
						}).Info("Past live stream")
						NewYoutubeData.UpdateStatus(config.PastStatus)
						notif.SendDude(NewYoutubeData, Bot)
					}
				}
			}
		}
	}
}

//YtAPI Get data from youtube api
func YtAPI(VideoID []string) (YtData, error) {
	var (
		Data YtData
	)

	body, curlerr := network.Curl("https://www.googleapis.com/youtube/v3/videos?part=statistics,snippet,liveStreamingDetails,contentDetails&fields=items(snippet(publishedAt,title,description,thumbnails(standard),channelTitle,liveBroadcastContent),liveStreamingDetails(scheduledStartTime,concurrentViewers,actualEndTime),statistics(viewCount),contentDetails(duration))&id="+strings.Join(VideoID, ",")+"&key="+*yttoken, nil)
	if curlerr != nil {
		log.Error(curlerr)
	}
	err := json.Unmarshal(body, &Data)
	if err != nil {
		return Data, err
	}

	return Data, nil
}

//ParseDuration Parse video duration
func ParseDuration(str string) time.Duration {
	durationRegex := regexp.MustCompile(`P(?P<years>\d+Y)?(?P<months>\d+M)?(?P<days>\d+D)?T?(?P<hours>\d+H)?(?P<minutes>\d+M)?(?P<seconds>\d+S)?`)
	matches := durationRegex.FindStringSubmatch(str)

	years := ParseInt64(matches[1])
	months := ParseInt64(matches[2])
	days := ParseInt64(matches[3])
	hours := ParseInt64(matches[4])
	minutes := ParseInt64(matches[5])
	seconds := ParseInt64(matches[6])

	hour := int64(time.Hour)
	minute := int64(time.Minute)
	second := int64(time.Second)
	return time.Duration(years*24*365*hour + months*30*24*hour + days*24*hour + hours*hour + minutes*minute + seconds*second)
}

func ParseInt64(value string) int64 {
	if len(value) == 0 {
		return 0
	}
	parsed, err := strconv.Atoi(value[:len(value)-1])
	if err != nil {
		return 0
	}
	return int64(parsed)
}

package dapr

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/dapr/go-sdk/client"
	"github.com/dapr/go-sdk/service/common"
	daprd "github.com/dapr/go-sdk/service/grpc"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/storage/pkg/model"
)

var (
	Err = "Error"
)

type Mode struct {
	ID       int
	IsActive int
}

type PubSub struct {
	client      client.Client
	Logger      *logrus.Logger
	Timestamp   time.Time
	mode        Mode
	Description string
	Requests    int64
	pubsubName  string
	pubTopic    string
	db          *gorm.DB
	mtx         sync.RWMutex
	mtxDescr    sync.RWMutex
}

type Message struct {
	Id   uuid.UUID
	Data string
}

func InitPubsub(description string, pubsubName string, subPort string, pubPort string,
	logger *logrus.Logger, done chan struct{}, db *gorm.DB) (*PubSub, error) {
	var err error
	clientPub, err := client.NewClientWithPort(pubPort)
	if err != nil {
		logger.Fatal(err)
	}

	mode := Mode{}
	db.Attrs(Mode{ID: 1, IsActive: 1}).FirstOrCreate(&mode, Mode{})

	pubSub := PubSub{
		Logger:      logger,
		Description: description,
		client:      clientPub,
		Timestamp:   time.Now(),
		Requests:    0,
		mode:        mode,
		db:          db,
		pubsubName:  pubsubName,
		pubTopic:    viper.GetString("dapr.publish.topic"),
	}
	s, err := daprd.NewService(subPort)
	if err != nil {
		log.Fatalf("failed to start the server: %v", err)
	}

	sub := &common.Subscription{
		PubsubName: pubsubName,
		Topic:      model.GetDescriptionTopic,
	}
	if err := s.AddTopicEventHandler(sub, pubSub.infoHandler); err != nil {
		log.Fatalf("error adding invocation handler: %v", err)
	}
	sub.Topic = model.GetUptimeTopic
	if err := s.AddTopicEventHandler(sub, pubSub.uptimeHandler); err != nil {
		log.Fatalf("error adding invocation handler: %v", err)
	}
	sub.Topic = model.GetRequestsTopic
	if err := s.AddTopicEventHandler(sub, pubSub.requestsHandler); err != nil {
		log.Fatalf("error adding invocation handler: %v", err)
	}
	sub.Topic = model.UpdateDescriptionTopic
	if err := s.AddTopicEventHandler(sub, pubSub.updateInfoHandler); err != nil {
		log.Fatalf("error adding invocation handler: %v", err)
	}
	sub.Topic = model.GetModeTopic
	if err := s.AddTopicEventHandler(sub, pubSub.getModeHandler); err != nil {
		log.Fatalf("error adding invocation handler: %v", err)
	}
	sub.Topic = model.SetModeTopic
	if err := s.AddTopicEventHandler(sub, pubSub.setModeHandler); err != nil {
		log.Fatalf("error adding invocation handler: %v", err)
	}
	sub.Topic = model.RestartTopic
	if err := s.AddTopicEventHandler(sub, pubSub.restartHandler); err != nil {
		log.Fatalf("error adding invocation handler: %v", err)
	}
	go func() {
		if err := s.Start(); err != nil {
			done <- struct{}{}
			log.Fatalf("server error: %v", err)
		}
	}()
	return &pubSub, nil
}

func (p *PubSub) infoHandler(ctx context.Context, e *common.TopicEvent) (retry bool, err error) {
	message := Message{}
	parsed, ok := e.Data.([]byte)
	if !ok {
		return false, nil
	}
	err = json.Unmarshal(parsed, &message)
	if err != nil {
		p.Logger.Debug(err)
		return false, err
	}
	message.Data = p.GetDescriptionFromServer()
	messageMarshal, err := json.Marshal(message)
	if err != nil {
		p.Logger.Debug(err)
		return false, err
	}
	p.client.PublishEvent(ctx, p.pubsubName, p.pubTopic, messageMarshal)
	p.IncRequests()
	log.Printf("%s", messageMarshal)
	return false, nil
}
func (p *PubSub) updateInfoHandler(ctx context.Context, e *common.TopicEvent) (retry bool, err error) {
	message := Message{}
	parsed, ok := e.Data.([]byte)
	if !ok {
		return false, nil
	}
	err = json.Unmarshal(parsed, &message)
	if err != nil {
		p.Logger.Debug(err)
		return false, err
	}
	p.UpdateDescriptionFromServer(message.Data)
	messageMarshal, err := json.Marshal(message)
	if err != nil {
		p.Logger.Debug(err)
		return false, err
	}
	p.client.PublishEvent(ctx, p.pubsubName, p.pubTopic, messageMarshal)
	p.IncRequests()
	log.Printf("%s", e.Data)
	return false, nil
}

func (p *PubSub) uptimeHandler(ctx context.Context, e *common.TopicEvent) (retry bool, err error) {
	uptime := time.Now().Unix() - p.Timestamp.Unix()
	message := Message{}
	parsed, ok := e.Data.([]byte)
	if !ok {
		return false, nil
	}
	err = json.Unmarshal(parsed, &message)
	if err != nil {
		p.Logger.Debug(err)
		return false, err
	}
	message.Data = fmt.Sprintf("%d", uptime)
	messageMarshal, err := json.Marshal(message)
	if err != nil {
		p.Logger.Debug(err)
		return false, err
	}
	p.client.PublishEvent(ctx, p.pubsubName, p.pubTopic, messageMarshal)
	p.IncRequests()
	log.Printf("%s", e.Data)
	return false, nil
}

func (p *PubSub) getModeHandler(ctx context.Context, e *common.TopicEvent) (retry bool, err error) {
	message := Message{}
	parsed, ok := e.Data.([]byte)
	if !ok {
		return false, nil
	}
	err = json.Unmarshal(parsed, &message)
	if err != nil {
		p.Logger.Debug(err)
		return false, err
	}
	mode := Mode{}
	p.db.First(&mode)
	message.Data = fmt.Sprintf("%d", mode.IsActive)
	messageMarshal, err := json.Marshal(message)
	if err != nil {
		p.Logger.Debug(err)
		return false, err
	}
	p.client.PublishEvent(ctx, p.pubsubName, p.pubTopic, messageMarshal)
	p.IncRequests()
	log.Printf("%s", e.Data)
	return false, nil
}

func (p *PubSub) setModeHandler(ctx context.Context, e *common.TopicEvent) (retry bool, err error) {
	message := Message{}
	parsed, ok := e.Data.([]byte)
	if !ok {
		return false, err
	}
	err = json.Unmarshal(parsed, &message)
	if err != nil {
		p.Logger.Debug(err)
		return false, err
	}
	p.mode.IsActive, err = strconv.Atoi(message.Data)
	if err != nil {
		return false, err
	}
	p.db.Save(p.mode)
	messageMarshal, err := json.Marshal(message)
	if err != nil {
		p.Logger.Debug(err)
		return false, err
	}
	p.client.PublishEvent(ctx, p.pubsubName, p.pubTopic, messageMarshal)
	p.IncRequests()
	log.Printf("%s", e.Data)
	return false, nil
}

func (p *PubSub) requestsHandler(ctx context.Context, e *common.TopicEvent) (retry bool, err error) {
	message := Message{}
	parsed, ok := e.Data.([]byte)
	if !ok {
		return false, err
	}
	err = json.Unmarshal(parsed, &message)
	if err != nil {
		p.Logger.Debug(err)
		return false, err
	}
	message.Data = fmt.Sprintf("%d", p.GetRequestsFromServer())
	messageMarshal, err := json.Marshal(message)
	if err != nil {
		p.Logger.Debug(err)
		return false, err
	}
	p.client.PublishEvent(ctx, p.pubsubName, p.pubTopic, messageMarshal)
	p.IncRequests()
	log.Printf("%s", messageMarshal)
	return false, nil
}

func (p *PubSub) restartHandler(ctx context.Context, e *common.TopicEvent) (retry bool, err error) {
	p.Description = viper.GetString("app.id")
	p.Timestamp = time.Now()
	p.Requests = 0
	return false, nil
}

func (p *PubSub) GetRequestsFromServer() int {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	return int(p.Requests)
}

func (p *PubSub) IncRequests() {
	p.mtx.Lock()
	p.Requests++
	p.mtx.Unlock()
}

func (p *PubSub) GetDescriptionFromServer() string {
	p.mtxDescr.RLock()
	defer p.mtxDescr.RUnlock()
	return p.Description
}

func (p *PubSub) UpdateDescriptionFromServer(description string) {
	p.mtxDescr.Lock()
	p.Description = description
	p.mtxDescr.Unlock()
}

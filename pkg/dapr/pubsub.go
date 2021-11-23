package dapr

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dapr/go-sdk/client"
	"github.com/dapr/go-sdk/service/common"
	daprd "github.com/dapr/go-sdk/service/grpc"
	"github.com/sirupsen/logrus"
)

type PubSub struct {
	client      client.Client
	Logger      *logrus.Logger
	Timestamp   time.Time
	Description string
	Requests    int64
}

func InitPubsub(description string, pubsubName string, subPort string, pubPort string, logger *logrus.Logger, done chan struct{}) (*PubSub, error) {
	var err error
	clientPub, err := client.NewClientWithPort(pubPort)
	if err != nil {
		logger.Fatal(err)
	}
	pubSub := PubSub{
		Logger:      logger,
		Description: description,
		client:      clientPub,
		Timestamp:   time.Now(),
		Requests:    0,
	}
	s, err := daprd.NewService(subPort)
	if err != nil {
		log.Fatalf("failed to start the server: %v", err)
	}
	sub := &common.Subscription{
		PubsubName: "messages",
		Topic:      "info",
	}
	if err := s.AddTopicEventHandler(sub, pubSub.infoHandler); err != nil {
		log.Fatalf("error adding invocation handler: %v", err)
	}
	sub.Topic = "uptime"
	if err := s.AddTopicEventHandler(sub, pubSub.uptimeHandler); err != nil {
		log.Fatalf("error adding invocation handler: %v", err)
	}
	sub.Topic = "requests"
	if err := s.AddTopicEventHandler(sub, pubSub.requestsHandler); err != nil {
		log.Fatalf("error adding invocation handler: %v", err)
	}
	sub.Topic = "update-info"
	if err := s.AddTopicEventHandler(sub, pubSub.updateInfoHandler); err != nil {
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
	p.client.PublishEvent(ctx, "messages", "respond", []byte(p.Description))
	p.Requests += 1
	log.Printf("%s", e.Data)
	return false, nil
}
func (p *PubSub) updateInfoHandler(ctx context.Context, e *common.TopicEvent) (retry bool, err error) {
	p.Description = fmt.Sprintf("%s", e.Data)
	p.client.PublishEvent(ctx, "messages", "respond", []byte(p.Description))
	p.Requests += 1
	log.Printf("%s", e.Data)
	return false, nil
}

func (p *PubSub) uptimeHandler(ctx context.Context, e *common.TopicEvent) (retry bool, err error) {
	uptime := time.Now().Unix() - p.Timestamp.Unix()
	p.client.PublishEvent(ctx, "messages", "respond", []byte(fmt.Sprintf("%d", uptime)))
	p.Requests += 1
	log.Printf("%s", e.Data)
	return false, nil
}

func (p *PubSub) requestsHandler(ctx context.Context, e *common.TopicEvent) (retry bool, err error) {
	p.client.PublishEvent(ctx, "messages", "respond", []byte(fmt.Sprintf("%d", p.Requests)))
	p.Requests += 1
	log.Printf("%s", e.Data)
	return false, nil
}

package attacker

import (
	"github.com/sirupsen/logrus"
	"github.com/tsinghua-cel/attacker-service/attackclient"
)

var (
	serviceUrl string
	client     *attackclient.Client
)

func InitAttacker(url string) error {
	serviceUrl = url
	logrus.WithField("url", serviceUrl).Info("Attacker service init")
	return nil
}

func GetAttacker() *attackclient.Client {
	if client != nil {
		return client
	}
	if serviceUrl == "" {
		return nil
	}

	c, err := attackclient.Dial(serviceUrl)
	if err != nil {
		return nil
	}
	client = c
	return client
}

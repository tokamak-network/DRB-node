package utils

import (
	"encoding/json"
	"io/ioutil"

	"github.com/fatih/color"
	"github.com/sirupsen/logrus"
)

type Config struct {
	RpcURL          string `json:"RpcURL"`
	HttpURL         string `json:"HttpURL"`
	PrivateKey      string `json:"PrivateKey"`
	ContractAddress string `json:"ContractAddress"`
	WalletAddress   string `json:"WalletAddress"`
	SubgraphURL     string `json:"SubgraphURL"`
}

func LoadConfig() Config {
	var cfg Config
	configFile, err := ioutil.ReadFile("../config.json")
	if err != nil {
		logrus.Fatalf("Error reading config file: %s", err)
	}
	if err := json.Unmarshal(configFile, &cfg); err != nil {
		logrus.Fatalf("Error parsing config file: %s", err)
	}
	return cfg
}

func PrintLogo() {
	color.Cyan(`
                  _____          ___                  ___           ___          _____          ___     
      ___        /  /::\        /  /\                /__/\         /  /\        /  /::\        /  /\    
     /__/\      /  /:/\:\      /  /:/_               \  \:\       /  /::\      /  /:/\:\      /  /:/_   
     \  \:\    /  /:/  \:\    /  /:/ /\               \  \:\     /  /:/\:\    /  /:/  \:\    /  /:/ /\  
      \  \:\  /__/:/ \__\:|  /  /:/ /:/           _____\__\:\   /  /:/  \:\  /__/:/ \__\:|  /  /:/ /:/_ 
  ___  \__\:\ \  \:\ /  /:/ /__/:/ /:/           /__/::::::::\ /__/:/ \__\:\ \  \:\ /  /:/ /__/:/ /:/ /\
 /__/\ |  |:|  \  \:\  /:/  \  \:\/:/            \  \:\~~\~~\/ \  \:\ /  /:/  \  \:\  /:/  \  \:\/:/ /:/
 \  \:\|  |:|   \  \:\/:/    \  \::/              \  \:\  ~~~   \  \:\  /:/    \  \:\/:/    \  \::/ /:/ 
  \  \:\__|:|    \  \::/      \  \:\               \  \:\        \  \:\/:/      \  \::/      \  \:\/:/  
   \__\::::/      \__\/        \  \:\               \  \:\        \  \::/        \__\/        \  \::/   
                                \__\/                \__\/         \__\/                       \__\/    
	`)
}

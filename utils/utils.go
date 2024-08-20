package utils

import (
	"encoding/json"
	"io/ioutil"

	"github.com/fatih/color"
	"github.com/tokamak-network/DRB-Node/logger" // Import your logger package
)

type Config struct {
	RpcURL          string `json:"RpcURL"`
	HttpURL         string `json:"HttpURL"`
	ContractAddress string `json:"ContractAddress"`
	SubgraphURL     string `json:"SubgraphURL"`
}

func LoadConfig() Config {
	var cfg Config
	configFile, err := ioutil.ReadFile("./config.json")
	if err != nil {
		logger.Log.Fatalf("Error reading config file: %s", err)
	}
	if err := json.Unmarshal(configFile, &cfg); err != nil {
		logger.Log.Fatalf("Error parsing config file: %s", err)
	}
	return cfg
}

func PrintLogo() {
	color.Cyan(`
              ___           ___          _____          ___     
             /__/\         /  /\        /  /::\        /  /\    
             \  \:\       /  /::\      /  /:/\:\      /  /:/_   
              \  \:\     /  /:/\:\    /  /:/  \:\    /  /:/ /\  
        _____  \__\:\   /  /:/  \:\  /__/:/ \__\:|  /  /:/ /:/_ 
         /__/::::::::\ /__/:/ \__\:\ \  \:\ /  /:/ /__/:/ /:/ /\
    	 \  \:\~~\~~\/ \  \:\ /  /:/  \  \:\  /:/  \  \:\/:/ /:/
       	  \  \:\  ~~~   \  \:\  /:/    \  \:\/:/    \  \::/ /:/ 
           \  \:\        \  \:\/:/      \  \::/      \  \:\/:/  
        	\  \:\        \  \::/        \__\/        \  \::/   
             \__\/         \__\/                       \__\/    
	`)
}

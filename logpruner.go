package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/juju/deputy"
	"github.com/spf13/viper"
	"log"
	"os"
	"os/exec"
	"time"
)

// Allow logging of debug information (see: https://gist.github.com/a53mt/60c1002955e6d3096078).
const debug debugging = true // or flip to false
type debugging bool

func (d debugging) Printf(format string, args ...interface{}) {
	if d {
		log.Printf("DEBUG  "+format, args...)
	}
}

// The Docker image to run.
const DOCKER_IMAGE_TO_RUN string = "my/logpruner:0.0.1"

// Struct to hold the ClouWatch describe-alarms JSON response.
// Generated via JSONGen (https://github.com/bemasher/JSONGen).
type AlarmFreeLogSpace struct {
	MetricAlarms []struct {
		ActionsEnabled                     bool
		AlarmActions                       []string
		AlarmArn                           string
		AlarmConfigurationUpdatedTimestamp string
		AlarmDescription                   string
		AlarmName                          string
		ComparisonOperator                 string
		Dimensions                         []struct {
			Name  string
			Value string
		}
		EvaluationPeriods       int64
		InsufficientDataActions []interface{}
		MetricName              string
		Namespace               string
		OKActions               []interface{}
		Period                  int64
		StateReason             string
		StateReasonData         string
		StateUpdatedTimestamp   string
		StateValue              string
		Statistic               string
		Threshold               float64
	}
}

// Holding all the required information to run AWS cli and ElasticSearch curator commands.
type LogprunerCfg struct {
	// Needed for AWS cli describe-alarms.
	AlarmName string `mapstructure:"alarm_name"`
	// Needed for ElasticSearch curator.
	Host          string `mapstructure:"host"`
	Port          int    `mapstructure:"port"`
	OlderThanDays int    `mapstructure:"older_than_days"`
	// The zero value for bool is false.
	UseSSL        bool `mapstructure:"use_SSL"`
	SSLValidation bool `mapstructure:"ssl_validation"`
}

// Check if we are operable at all.
func init() {
	// Environment vars check.
	envVarsToChk := []string{"AWS_DEFAULT_REGION", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"}
	for _, ev := range envVarsToChk {
		if _, err := getEnvVarOrErr(ev); err != nil {
			log.Fatalf("Error: %s. Exiting now.\n", err.Error())
		}
	}
	// Config file check.
	configDir := "/etc/logpruner"
	if _, err := os.Stat(configDir); err != nil {
		if os.IsNotExist(err) {
			log.Fatalf("Error: Configuration directory '%s' does not exist. Exiting now.\n", configDir)
		} else {
			// TODO Check other possible errors (permissions etc.)
		}
	}
	// Change into config dir.
	if err := os.Chdir(configDir); err != nil {
		log.Fatalf("Error: Could not change into config dir '%s'. %s\n", configDir, err.Error())
	}
	viper.SetConfigName("logpruner_config") // name of config file (without extension)
	viper.AddConfigPath(configDir)          // path to look for the config file in
	err := viper.ReadInConfig()             // Find and read the config file
	if err != nil {                         // Handle errors reading the config file
		log.Fatalf("Error: Unable to read config file: %s", err.Error())
	}
}

// Helper function spitting out the CLI syntax for ES curator.
func (lpc LogprunerCfg) renderForCuratorDeleteIndexAction() string {
	// curator --host ess-endpoint.live.hellofresh.io --port 82 delete indices --older-than 5 --time-unit days --timestring '%%Y.%%m.%d'
	res := "curator "
	res = res + fmt.Sprintf("--host %s --port %d",
		lpc.Host,
		lpc.Port)
	// Handle boolean values.
	if lpc.UseSSL {
		res = res + " --use_ssl"
		// Per default SSL validation happens.
		if !lpc.SSLValidation {
			res = res + " --ssl-no-validate"
		}
	}
	res = res + fmt.Sprintf(" delete indices --older-than %d --time-unit days", lpc.OlderThanDays)

	res = res + " " + "--timestring '%Y.%m.%d'"
	return res
}

// Helper function to retrieve the value of an OS environment variable. If not set or empty, return error.
func getEnvVarOrErr(varName string) (string, error) {
	switch envVar := os.Getenv(varName); envVar {
	case "":
		return "", fmt.Errorf("Required environment variable '%s' unset or empty.\n", varName)
	default:
		return envVar, nil
	}
}

// Read the config values into typed struct values.
func retrieveCfgVals() (map[string]*LogprunerCfg, error) {
	// Containing the retrieved config values in a typed manner.
	var cfgVals map[string]*LogprunerCfg = make(map[string]*LogprunerCfg)

	// Iterate over all defined indexes.
	indexesMap := viper.GetStringMapString("es_indexes")
	for idxName, _ := range indexesMap {
		// Create new struct container holding the config vals. The map key equals the index name defined on the
		// 2.nd level in the YAML configuration file.
		cfgVals[idxName] = &LogprunerCfg{}
		if err := viper.UnmarshalKey("es_indexes"+"."+idxName, cfgVals[idxName]); err != nil {
			return nil, fmt.Errorf("Error unmarshalling config values into LogprunerCfg struct: %s", err.Error())
		}
	}
	return cfgVals, nil
}

// Using AWS cli tool to retrieve an alarm with the given name.
func getCloudWatchAlarm(alarmName string) (string, error) {
	cmdStdoutPipeBuffer := bytes.NewBuffer(nil)
	d := deputy.Deputy{
		Errors: deputy.FromStderr,
		// Capture the cmd output into cmdStdOutPipeBuffer.
		StdoutLog: func(b []byte) {
			cmdStdoutPipeBuffer.WriteString(string(b))
		},
		Timeout: time.Second * 30,
	}

	cmd := exec.Command("docker",
		"run",
		"--rm",
		"-e",
		fmt.Sprintf("AWS_DEFAULT_REGION=%s", os.Getenv("AWS_DEFAULT_REGION")),
		"-e",
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", os.Getenv("AWS_ACCESS_KEY_ID")),
		"-e",
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", os.Getenv("AWS_SECRET_ACCESS_KEY")),
		"-i",
		DOCKER_IMAGE_TO_RUN,
		// Because the ENTRYPOINT of the Docker image to run is already "/bin/sh" we only have to provide "-c" here
		// to read the commands to execute from the command line.
		"-c",
		fmt.Sprintf("aws cloudwatch describe-alarms --alarm-names %s", alarmName))
	if err := d.Run(cmd); err != nil {
		return "", fmt.Errorf("(getCloudWatchAlarm) >>  Error executing docker run cmd. Error: %s\n", err.Error())
	}
	return cmdStdoutPipeBuffer.String(), nil
}

// Uses ElasticSearch curator tool to delete old indexes.
func deleteESIndex(logrunerCfg *LogprunerCfg) error {
	d := deputy.Deputy{
		Errors:  deputy.FromStderr,
		Timeout: time.Second * 120,
	}
	cmd := exec.Command("docker",
		"run",
		"--rm",
		"-e",
		fmt.Sprintf("AWS_DEFAULT_REGION=%s", os.Getenv("AWS_DEFAULT_REGION")),
		"-e",
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", os.Getenv("AWS_ACCESS_KEY_ID")),
		"-e",
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", os.Getenv("AWS_SECRET_ACCESS_KEY")),
		"-i",
		DOCKER_IMAGE_TO_RUN,
		// Because the ENTRYPOINT of the Docker image to run is already "/bin/sh" we only have to provide "-c" here
		// to read the commands to execute from the command line.
		"-c",
		logrunerCfg.renderForCuratorDeleteIndexAction())

	debug.Printf("(deleteESIndex)  'cmd': %v\n", cmd.Args)
	if err := d.Run(cmd); err != nil {
		return fmt.Errorf("(deleteESIndex) >>  Error executing docker run cmd. Error: %s\n", err.Error())
	}
	return nil
}

func isDeleteActionRequired(alarmDesc *AlarmFreeLogSpace) (bool, error) {
	switch alarmState := alarmDesc.MetricAlarms[0].StateValue; alarmState {
	case "OK":
		return false, nil
	case "ALARM":
		return true, nil
	default:
		return false, fmt.Errorf("Unknown StateValue '%s'. Do not know how to handle.\n", alarmState)
	}
}

func main() {
	// Collect config values.
	if cfgVals, err := retrieveCfgVals(); err != nil {
		log.Fatalf(err.Error())
	} else {
		// Print collected config values.
		for idxName, lpCfg := range cfgVals {
			fmt.Printf("==> Retrieving alarm values for index: '%s' and alarm name: '%s'\n", idxName, lpCfg.AlarmName)
			debug.Printf("============================================================\n")
			debug.Printf("    %#v\n", lpCfg)
			debug.Printf("============================================================\n")
			cloudWatchAlarmJSON, err := getCloudWatchAlarm(lpCfg.AlarmName)
			if err != nil {
				log.Println(err.Error())
			} else {
				debug.Printf("cloudWatchAlarm: '%s'\n", cloudWatchAlarmJSON)
				var alarmDesc AlarmFreeLogSpace

				if err := json.Unmarshal([]byte(cloudWatchAlarmJSON), &alarmDesc); err != nil {
					log.Fatalf("Error unmarshalling AWS CloudWatch response JSON: %s\n", err.Error())
				}
				log.Printf("AlarmName: '%s'\n", alarmDesc.MetricAlarms[0].AlarmName)
				log.Printf("AlarmArn: '%s'\n", alarmDesc.MetricAlarms[0].AlarmArn)
				log.Printf("StateValue: '%s'\n", alarmDesc.MetricAlarms[0].StateValue)
				if delActnReq, err := isDeleteActionRequired(&alarmDesc); err != nil {
					log.Fatalf(err.Error())
				} else {
					debug.Printf("***  DELETE ACTION REQUIRED? %t  ***", delActnReq)
					// Let's delete some old ElasticSearch indexes.
					if delActnReq {
						log.Printf(">>>  TRIGGERING DELETE OLD INDEXES ACTION for index '%s'  <<<", idxName)
						if err := deleteESIndex(lpCfg); err != nil {
							log.Printf("Could not delete old indexes for '%s' at host '%s', port %d. Error: %s\n", idxName, lpCfg.Host, lpCfg.Port, err.Error())
						} else {
							log.Printf("Successfully deleted old indexes for '%s' at host '%s', port %d.\n", idxName, lpCfg.Host, lpCfg.Port)
						}
					}
				}

			}
		}
	}

}

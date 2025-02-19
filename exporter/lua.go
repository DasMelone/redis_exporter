package exporter

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gomodule/redigo/redis"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

func (e *Exporter) extractLuaScriptMetrics(ch chan<- prometheus.Metric, c redis.Conn, filename string, script []byte) error {
	log.Debugf("Evaluating e.options.LuaScript: %s", filename)
	kv, err := redis.StringMap(doRedisCmd(c, "EVAL", script, 0, 0))
	if err != nil {
		log.Errorf("LuaScript error: %v", err)
		e.registerConstMetricGauge(ch, "script_result", 0, filename)
		return err
	}

	if len(kv) == 0 {
		log.Debugf("Lua script returned no results")
		e.registerConstMetricGauge(ch, "script_result", 2, filename)
		return nil
	}

	for key, stringVal := range kv {
		keyfm := strings.Split(key, "+++")
		key := keyfm[0]
		var fm string
		if len(keyfm) > 1 {
			fm = keyfm[1]
		} else {
			fm = ""
		}

		//special cases
		if key == "metrics:players:skin" {
			parsedData, err := parseJSON(stringVal)
			if err != nil {
				log.Debugf("metrics:players:skin script returned ivalid json")
				e.registerConstMetricGauge(ch, "script_result", 2, filename)
				continue
			}
			e.registerAsValue(ch, boolToFloat(parsedData["cape"].(bool)), "metrics:players:skin++cape", filename, fm)
			e.registerAsValue(ch, boolToFloat(parsedData["jacket"].(bool)), "metrics:players:skin++jacket", filename, fm)
			e.registerAsValue(ch, boolToFloat(parsedData["hat"].(bool)), "metrics:players:skin++hat", filename, fm)
			e.registerAsValue(ch, boolToFloat(parsedData["right_pants"].(bool)), "metrics:players:skin++right_pants", filename, fm)
			e.registerAsValue(ch, boolToFloat(parsedData["left_pants"].(bool)), "metrics:players:skin++left_pants", filename, fm)
			e.registerAsValue(ch, boolToFloat(parsedData["left_sleeve"].(bool)), "metrics:players:skin++left_sleeve", filename, fm)
			e.registerAsValue(ch, boolToFloat(parsedData["right_sleeve"].(bool)), "metrics:players:skin++right_sleeve", filename, fm)
			e.registerConstMetricGauge(ch, "script_result", 1, filename)
			continue
		} else if key == "bungee:servers:state" {
			enum := map[string]float64{
				"INVISIBLE":   1,
				"MAP":         2,
				"MAP_EMPTY":   3,
				"MAP_FULL":    4,
				"LOBBY":       5,
				"LOBBY_EMPTY": 6,
				"LOBBY_FULL":  7,
				"STARTING":    8,
				"INGAME":      9,
				"ENDING":      10,
			}
			e.registerAsValue(ch, enum[stringVal], "bungee:servers:state", filename, fm)
			continue
		} else if key == "lobby:visibility" {
			enum := map[string]float64{
				"ALL":  1,
				"TEAM": 2,
				"NONE": 3,
			}
			e.registerAsValue(ch, enum[stringVal], "lobby:visibility", filename, fm)
			continue
		} else if key == "metrics:players:chatmode" {
			enum := map[string]float64{
				"SHOWN":         1,
				"COMMANDS_ONLY": 2,
				"HIDDEN":        3,
			}
			e.registerAsValue(ch, enum[stringVal], "metrics:players:chatmode", filename, fm)
			continue
		} else if key == "metrics:players:locale" {
			enum := map[string]float64{
				"en_US": 1,
				"de_DE": 2,
			}
			locale, ok := enum[stringVal]
			if !ok {
				locale = 0 // Zero for unknown locale
			}
			e.registerAsValue(ch, locale, "metrics:players:locale", filename, fm)
			continue
		} else if key == "metrics:players:server" {
			enum := map[string]float64{
				"Lobby":    1,
				"Survival": 2,
				"KFFA-1":   3,
				"Spread-1": 4,
				"Spread-2": 5,
				"Spread-3": 6,
			}
			e.registerAsValue(ch, enum[stringVal], "metrics:players:server", filename, fm)
			continue
		} else if key == "metrics:players:version:name" {
			cleanedString := strings.ReplaceAll(stringVal, ".", "")

			result, err := strconv.ParseFloat(cleanedString, 64)
			if err != nil {
				fmt.Println("Error:", err)
				continue
			}
			e.registerAsValue(ch, result, "metrics:players:version:name", filename, fm)
			continue
		} else if key == "players:chat:target" {
			enum := map[string]float64{
				"NORMAL": 1,
				"TEAM":   2,
				"ADMIN":  3,
			}
			e.registerAsValue(ch, enum[stringVal], "players:chat:target", filename, fm)
			continue
		} else if strings.HasPrefix(key, "metrics:players:ip") || strings.HasPrefix(key, "metrics:players:labymod") || strings.HasPrefix(key, "moderation") || strings.HasPrefix(key, "metrics:players:modded") || key == "metrics:players:version:brand" || key == "metrics:players:name" {
			continue
		}

		if val, err := strconv.ParseFloat(stringVal, 64); err == nil {
			// Only record value metric if value is float-y
			e.registerAsValue(ch, val, key, filename, fm)
		} else {
			if key == "weather:skip" {
				log.Errorf("weather:skip failed to parse as float: %v", err)
			}

			// if it's not float-y then we'll try to interprete the value as a float
			if stringVal == "true" {
				val := 1.0
				e.registerAsValue(ch, val, key, filename, fm)
			} else if stringVal == "false" {
				val := 0.0
				e.registerAsValue(ch, val, key, filename, fm)
			} else {
				// if it's really not float-y then we'll record the value as a string label
				e.registerAsString(ch, stringVal, key, filename, fm)
			}
		}
	}
	e.registerConstMetricGauge(ch, "script_result", 1, filename)
	return nil
}

func (e *Exporter) registerAsValue(ch chan<- prometheus.Metric, val float64, key string, filename string, fm string) {
	if fm != "" {
		e.registerConstMetricGauge(ch, "script_values", val, key, filename, fm)
	} else {
		e.registerConstMetricGauge(ch, "script_values", val, key, filename)
	}
}

func (e *Exporter) registerAsString(ch chan<- prometheus.Metric, val string, key string, filename string, fm string) {
	/* if fm != "" {
		e.registerConstMetricGauge(ch, "script_values_as_string", 1.0, key, filename, val, fm)
	} else {
		e.registerConstMetricGauge(ch, "script_values_as_string", 1.0, key, filename, val)
	} */
}

func parseJSON(jsonStr string) (map[string]interface{}, error) {
	var data map[string]interface{}
	err := json.Unmarshal([]byte(jsonStr), &data)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func boolToFloat(val bool) float64 {
	if val {
		return 1.0
	}

	return 0.0
}

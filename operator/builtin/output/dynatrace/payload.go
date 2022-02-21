package dynatrace

import (
	"github.com/observiq/stanza/entry"
	"strconv"
)

// LogPayloadFromEntries creates a new []*LogPayload from an array of entries
func LogPayloadFromEntries(entries []*entry.Entry, messageField entry.Field,clusterid string) []map[string]string {
	logs := make([]map[string]string, 0, len(entries))
	for _, entry := range entries {
		logs = append(logs, LogMessageFromEntry(entry, messageField, clusterid))
	}



	return logs
}

// LogPayload represents a single payload delivered to the New Relic Log API
type LogPayload []struct {
	Logs   []*LogMessage
}



// LogMessageFromEntry creates a new LogMessage from a given entry.Entry
func LogMessageFromEntry(entry *entry.Entry, messageField entry.Field, clusterid string) map[string]string {
	logMessage :=make(map[string]string)

    if clusterid != "" {
        logMessage["dt.kubernetes.cluster.id"]=clusterid
    }

    logMessage["timestamp"] = strconv.FormatInt(entry.Timestamp.UnixNano() / 1000 / 1000,10)

	var message string
	err := entry.Read(messageField, &message)
    if err == nil {
        logMessage["content"] = message
    } else {
        if rec, ok := entry.Record.(map[string]interface{}); ok {
            for key, value:= range rec {
                logMessage["content"]+=","
                logMessage["content"]+=key
                logMessage["content"]+=":"
                v, ok:=value.(string)
                if ok {
                    logMessage["content"]+=v
                }

            }
        }
    }

    logMessage["severity"] = entry.Severity.String()
    for key, value := range entry.Labels  {
        logMessage[key] = value
    }
    for key, value := range entry.Resource {
        logMessage[key] = value
    }



	return logMessage
}

// LogMessage represents a single log entry that will be marshalled
// in the format expected by the New Relic Log API
type LogMessage struct {
	// Milliseconds or seconds since epoch
	Log map[string]string
}

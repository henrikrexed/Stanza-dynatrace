package dynatrace

import (
	"github.com/observiq/stanza/entry"
	"github.com/observiq/stanza/version"
)

// LogPayloadFromEntries creates a new []*LogPayload from an array of entries
func LogPayloadFromEntries(entries []*entry.Entry, messageField entry.Field) LogPayload {
	logs := make([]*LogMessage, 0, len(entries))
	for _, entry := range entries {
		logs = append(logs, LogMessageFromEntry(entry, messageField))
	}

	lp := LogPayload{{
		Logs: logs,
	}}

	return lp
}

// LogPayload represents a single payload delivered to the New Relic Log API
type LogPayload []struct {
	Logs   []*LogMessage    `json:"logs"`
}



// LogMessageFromEntry creates a new LogMessage from a given entry.Entry
func LogMessageFromEntry(entry *entry.Entry, messageField entry.Field) *LogMessage {
	logMessage := &LogMessage{
		Timestamp:  entry.Timestamp.UnixNano() / 1000 / 1000, // Convert to millis
		Log: make(map[string]interface{}),
	}

	var message string
	err := entry.Read(messageField, &message)

    if err == nil {
        logMessage.content = message
    }
    else
        logMessage.content = entry.Record

    logMessage.severity = entry.Severity.String()
    for key, value := range entry.Labels
    {
        logMessage.Log[key] = value
    }
    for key, value := range entry.Resource
    {
        logMessage.Log[key] = value
    }

	for key, value := range entry.Record
    {
        if key!="message"
            logMessage.Log[key] = value
    }


	return logMessage
}

// LogMessage represents a single log entry that will be marshalled
// in the format expected by the New Relic Log API
type LogMessage struct {
	// Milliseconds or seconds since epoch
	Timestamp  int64                  `json:"timestamp,omitempty"`
	Log map[string]interface{} `json:"log,omitempty"`
	content    string                 `json:"content"`
	severity string  `json:"severity,omitempty"`
}

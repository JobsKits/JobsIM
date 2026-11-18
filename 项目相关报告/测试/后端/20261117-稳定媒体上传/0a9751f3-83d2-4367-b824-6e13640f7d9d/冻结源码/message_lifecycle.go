package jobsim

import "encoding/json"

func init() {
	migrations["message"] += `ALTER TABLE message.messages ADD COLUMN IF NOT EXISTS content_unavailable boolean NOT NULL DEFAULT false;`
}
func (a *App) messageMutationOperation(user, message, kind string, payload any) string {
	raw, _ := json.Marshal(payload)
	return kind + ":" + stringBase64(keyedDigest(a.Config.Key, "message-operation:"+user+":"+message, raw))
}

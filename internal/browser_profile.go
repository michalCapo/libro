package libro

import (
	"crypto/sha256"
	"fmt"
)

// browserScope keeps profiles stable across restarts and distinct across threads.
func browserScope(sid, appID string) string {
	for _, project := range sm.GetAllRunningApps(sid) {
		for _, app := range project.Apps {
			if app.ID == appID {
				return fmt.Sprintf("%x", sha256.Sum256([]byte(project.Name)))
			}
		}
	}
	// A detached panel must never inherit another thread's profile.
	return fmt.Sprintf("%x", sha256.Sum256([]byte("panel:"+appID)))
}

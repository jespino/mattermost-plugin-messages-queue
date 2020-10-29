package main

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/plugin"
)

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration

	postsWaitingForOnline map[string][]*model.Post
}

// ServeHTTP demonstrates a plugin that handles HTTP requests by greeting the world.
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	fmt.Println(p.postsWaitingForOnline[userID])
	if posts, ok := p.postsWaitingForOnline[userID]; ok && posts != nil {
		for _, post := range posts {
			p.API.CreatePost(post)
		}
		p.postsWaitingForOnline[userID] = nil
	}
	fmt.Fprint(w, "{}")
}

func (p *Plugin) OnActivate() error {
	p.postsWaitingForOnline = map[string][]*model.Post{}
	if err := p.API.RegisterCommand(createDeferCommand()); err != nil {
		return err
	}
	if err := p.API.RegisterCommand(createQueueCommand()); err != nil {
		return err
	}
	return nil
}

// See https://developers.mattermost.com/extend/plugins/server/reference/

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorhill/cronexpr"
	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/plugin"
)

type Queue struct {
	Name       string               `json:"name"`
	SpecSource string               `json:"spec_source"`
	Spec       *cronexpr.Expression `json:"-"`
	UserId     string               `json:"user_id"`
	ChannelId  string               `json:"channel_id"`
	Messages   []string             `json:"messages"`
}

type DeferredPost struct {
	Time time.Time   `json:"time"`
	Post *model.Post `json:"post"`
}

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration

	postsWaitingForOnline map[string][]*model.Post
	deferredPosts         []*DeferredPost
	Queues                map[string]*Queue
}

// ServeHTTP demonstrates a plugin that handles HTTP requests by greeting the world.
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if posts, ok := p.postsWaitingForOnline[userID]; ok && posts != nil {
		for _, post := range posts {
			p.API.CreatePost(post)
		}
		p.postsWaitingForOnline[userID] = nil
	}
	fmt.Fprint(w, "{}")
}

func (p *Plugin) OnActivate() error {
	err := p.RestoreWaitingForOnlinePosts()
	if err != nil {
		p.API.LogError("failed to restore \"waiting for online\" posts", "err", err.Error())
	}
	err = p.RestoreDeferredPosts()
	if err != nil {
		p.API.LogError("failed to restore \"deferred\" posts", "err", err.Error())
	}
	err = p.RestoreQueues()
	if err != nil {
		p.API.LogError("failed to restore \"queues\"", "err", err.Error())
	}
	if err := p.API.RegisterCommand(createDeferCommand()); err != nil {
		return err
	}
	if err := p.API.RegisterCommand(createQueueCommand()); err != nil {
		return err
	}
	return nil
}

func (p *Plugin) SaveQueues() error {
	data, err := json.Marshal(p.Queues)
	if err != nil {
		return err
	}
	p.API.KVSet("queues", data)
	return nil
}

func (p *Plugin) RestoreQueues() error {
	p.Queues = map[string]*Queue{}
	data, appErr := p.API.KVGet("queues")
	if appErr != nil {
		return appErr
	}
	err := json.Unmarshal(data, &p.Queues)
	if err != nil {
		p.Queues = map[string]*Queue{}
		return err
	}
	for _, queue := range p.Queues {
		scheduleSpec, nErr := cronexpr.Parse(queue.SpecSource)
		if nErr != nil {
			p.API.LogError("failed to parse \"queue schedule\" info", "err", nErr.Error())
		}
		queue.Spec = scheduleSpec

		var handleTimeout func()
		handleTimeout = func() {
			if len(queue.Messages) > 0 {
				_, err := p.API.CreatePost(&model.Post{
					UserId:    queue.UserId,
					ChannelId: queue.ChannelId,
					Message:   queue.Messages[0],
				})
				if err != nil {
					p.API.LogError("failed to send scheduled post", "err", err.Error())
				}
				queue.Messages = queue.Messages[1:]
				nErr := p.SaveQueues()
				if nErr != nil {
					p.API.LogError("failed to save \"queues\"", "err", err.Error())
				}
			}
			model.CreateTask(fmt.Sprintf("check queue %s", queue.Name), handleTimeout, queue.Spec.Next(time.Now()).Sub(time.Now()))
		}

		model.CreateTask(fmt.Sprintf("check queue %s", queue.Name), handleTimeout, queue.Spec.Next(time.Now()).Sub(time.Now()))
	}
	return nil
}

func (p *Plugin) SaveDeferredPosts() error {
	data, err := json.Marshal(p.deferredPosts)
	if err != nil {
		return err
	}
	p.API.KVSet("deferred-posts", data)
	return nil
}

func (p *Plugin) RestoreDeferredPosts() error {
	p.deferredPosts = []*DeferredPost{}
	data, appErr := p.API.KVGet("deferred-posts")
	if appErr != nil {
		return appErr
	}
	err := json.Unmarshal(data, &p.deferredPosts)
	if err != nil {
		p.deferredPosts = []*DeferredPost{}
		return err
	}
	finalDeferredPosts := []*DeferredPost{}
	for _, deferredPost := range p.deferredPosts {
		if deferredPost.Time.Before(time.Now()) {
			_, err := p.API.CreatePost(deferredPost.Post)
			if err != nil {
				p.API.LogError(err.Error())
			}

		} else {
			finalDeferredPosts = append(finalDeferredPosts, deferredPost)
		}
	}
	p.deferredPosts = finalDeferredPosts
	p.SaveDeferredPosts()
	return nil
}

func (p *Plugin) SaveWaitingForOnlinePosts() error {
	data, err := json.Marshal(p.postsWaitingForOnline)
	if err != nil {
		return err
	}
	p.API.KVSet("waiting-for-online", data)
	return nil
}

func (p *Plugin) RestoreWaitingForOnlinePosts() error {
	p.postsWaitingForOnline = map[string][]*model.Post{}
	data, appErr := p.API.KVGet("waiting-for-online")
	if appErr != nil {
		return appErr
	}
	err := json.Unmarshal(data, &p.postsWaitingForOnline)
	if err != nil {
		p.deferredPosts = []*DeferredPost{}
		return err
	}
	return nil
}

package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorhill/cronexpr"

	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/plugin"
)

const deferCommand = "defer-post"
const queueCommand = "messages-queue"

func createDeferCommand() *model.Command {
	return &model.Command{
		Trigger:          deferCommand,
		AutoComplete:     true,
		AutoCompleteDesc: "Defer a message to be sent later.",
		AutoCompleteHint: "[command]",
		AutocompleteData: getDeferAutocompleteData(),
	}
}

func createQueueCommand() *model.Command {
	return &model.Command{
		Trigger:          queueCommand,
		AutoComplete:     true,
		AutoCompleteDesc: "Manage messages queues. Create/Delete/Modify queues, and add messages to the queue.",
		AutoCompleteHint: "[command]",
		AutocompleteData: getQueueAutocompleteData(),
	}
}

func getQueueAutocompleteData() *model.AutocompleteData {
	queue := model.NewAutocompleteData("messages-queue", "[command]", "Defer a post message to some time later")

	// * |/messages-queue remove-message <queue-name> <position>| - Remove a message from the queue in the specified position
	// * |/messages-queue insert-message <queue-name> <position> <message>| - Add a new message to the queue in the speicified position

	create := model.NewAutocompleteData("create", "[queue-name] [schedule]", "Create a new queue")
	create.AddTextArgument("Name of the new queue", "[queue-name]", "")
	create.AddTextArgument("Schedule in cron format", "[schedule]", "")
	queue.AddCommand(create)

	deleteQueue := model.NewAutocompleteData("delete", "[queue-name]", "Delete a queue")
	deleteQueue.AddTextArgument("Name of the queue", "[queue-name]", "")
	queue.AddCommand(deleteQueue)

	list := model.NewAutocompleteData("list", "", "List queues")
	queue.AddCommand(list)

	listQueue := model.NewAutocompleteData("list-messages", "[queue-name]", "List pending messages in a queue")
	listQueue.AddTextArgument("Name of the queue", "[queue-name]", "")
	queue.AddCommand(listQueue)

	add := model.NewAutocompleteData("add-message", "[queue-name] [message]", "Add a message to the queue")
	add.AddTextArgument("Name of the new queue", "[queue-name]", "")
	add.AddTextArgument("Message to add to the queue", "[message]", "")
	queue.AddCommand(add)

	remove := model.NewAutocompleteData("remove-message", "[queue-name] [position] [message]", "Remove a message from the queue")
	remove.AddTextArgument("Name of the new queue", "[queue-name]", "")
	remove.AddTextArgument("Position of the message", "[position]", "")
	queue.AddCommand(remove)

	insert := model.NewAutocompleteData("insert-message", "[queue-name] [position] [message]", "Insert a message in a position in the queue")
	insert.AddTextArgument("Name of the new queue", "[queue-name]", "")
	insert.AddTextArgument("Position of the message", "[position]", "")
	insert.AddTextArgument("Message to insert in the queue", "[message]", "")
	queue.AddCommand(insert)

	help := model.NewAutocompleteData("help", "", "Get slash command help")
	queue.AddCommand(help)
	return queue
}

func getDeferAutocompleteData() *model.AutocompleteData {
	deferPost := model.NewAutocompleteData("defer-post", "[online|time] [message]", "Defer a post message to some time later")

	online := model.NewAutocompleteData("online", "[message]", "Send the message when the user is online (only valid for DMs)")
	online.AddTextArgument("Message to send", "[message]", "")
	deferPost.AddCommand(online)

	help := model.NewAutocompleteData("help", "", "Get slash command help")
	deferPost.AddCommand(help)
	return deferPost
}

func (p *Plugin) ExecuteCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	split := strings.Fields(args.Command)
	command := split[0]

	if command == "/"+deferCommand {
		return p.executeDeferCommand(c, args)
	}

	if command == "/"+queueCommand {
		return p.executeQueueCommand(c, args)
	}
	return &model.CommandResponse{}, nil
}

func (p *Plugin) executeQueueCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	split := strings.Fields(args.Command)
	if (len(split) == 2 && split[1] == "help") || len(split) == 1 {
		return p.executeQueueHelpCommand(c, args)
	}

	if !p.API.HasPermissionTo(args.UserId, model.PERMISSION_MANAGE_SYSTEM) {
		_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
			ChannelId: args.ChannelId,
			Message:   "Permission denied, only system admins can handle messages queues",
		})
		return &model.CommandResponse{}, nil
	}

	if split[1] == "create" {
		if len(split) < 4 {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "Not enough arguments to create the queue",
			})
			return &model.CommandResponse{}, nil
		}
		scheduleSpec, err := cronexpr.Parse(strings.Join(split[3:], " "))
		if err != nil {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "Unable to parse the schedule, please see the supported format in the help text",
			})
			return &model.CommandResponse{}, nil
		}
		p.Queues[split[2]] = &Queue{
			Name:       split[2],
			UserId:     args.UserId,
			SpecSource: strings.Join(split[3:], " "),
			Spec:       scheduleSpec,
			ChannelId:  args.ChannelId,
			Messages:   []string{},
		}
		nErr := p.SaveQueues()
		if nErr != nil {
			p.API.LogError(nErr.Error())
		}

		var handleTimeout func()
		handleTimeout = func() {
			queue, ok := p.Queues[split[2]]
			if !ok {
				return
			}
			if len(queue.Messages) > 0 {
				_, err := p.API.CreatePost(&model.Post{
					UserId:    queue.UserId,
					ChannelId: queue.ChannelId,
					Message:   queue.Messages[0],
				})
				if err != nil {
					p.API.LogError(err.Error())
				}
				queue.Messages = queue.Messages[1:]
				nErr := p.SaveQueues()
				if nErr != nil {
					p.API.LogError(nErr.Error())
				}
			}
			model.CreateTask(fmt.Sprintf("check queue %s", split[2]), handleTimeout, queue.Spec.Next(time.Now()).Sub(time.Now()))
		}

		model.CreateTask(fmt.Sprintf("check queue %s", split[2]), handleTimeout, scheduleSpec.Next(time.Now()).Sub(time.Now()))

		_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
			ChannelId: args.ChannelId,
			Message:   fmt.Sprintf("Scheduling a queue, next execution: %v", scheduleSpec.Next(time.Now())),
		})
		return &model.CommandResponse{}, nil
	}

	if split[1] == "list" {
		if len(p.Queues) == 0 {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "No queues defined yet",
			})
			return &model.CommandResponse{}, nil
		}

		queuesList := []string{}
		for _, queue := range p.Queues {
			nextMessage := "no messages in the queue"
			if len(queue.Messages) > 0 {
				nextMessage = queue.Messages[0]
			}
			queuesList = append(queuesList, fmt.Sprintf(" * %s\n  * channel id: %s\n  * schedule spec: %s\n  * next execution: %s\n  * next message: %s",
				queue.Name, queue.ChannelId, queue.SpecSource, queue.Spec.Next(time.Now()), nextMessage,
			))
		}

		sort.Slice(queuesList, func(i, j int) bool {
			return queuesList[i] < queuesList[j]
		})

		queuesList = append([]string{"#### List of queues:"}, queuesList...)
		_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
			ChannelId: args.ChannelId,
			Message:   strings.Join(queuesList, "\n"),
		})
		return &model.CommandResponse{}, nil
	}

	if split[1] == "delete" {
		if len(split) < 3 {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "Not enough arguments to delete queue",
			})
			return &model.CommandResponse{}, nil
		}
		if len(p.Queues) == 0 {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   fmt.Sprintf("Queue %s doesn't exist", split[2]),
			})
			return &model.CommandResponse{}, nil
		}

		_, ok := p.Queues[split[2]]
		if !ok {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   fmt.Sprintf("Queue %s doesn't exist", split[2]),
			})
			return &model.CommandResponse{}, nil
		}
		delete(p.Queues, split[2])
		nErr := p.SaveQueues()
		if nErr != nil {
			p.API.LogError(nErr.Error())
		}

		_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
			ChannelId: args.ChannelId,
			Message:   fmt.Sprintf("Queue %s deleted", split[2]),
		})
		return &model.CommandResponse{}, nil
	}

	if split[1] == "add-message" {
		if len(split) < 4 {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "Not enough arguments to add a message",
			})
			return &model.CommandResponse{}, nil
		}
		queue, ok := p.Queues[split[2]]
		if !ok {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   fmt.Sprintf("Unknown queue %s.", split[2]),
			})
			return &model.CommandResponse{}, nil
		}
		queue.Messages = append(queue.Messages, strings.Join(split[3:], " "))
		nErr := p.SaveQueues()
		if nErr != nil {
			p.API.LogError(nErr.Error())
		}
		_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
			ChannelId: args.ChannelId,
			Message:   "Message added to the queue",
		})
		return &model.CommandResponse{}, nil
	}

	if split[1] == "remove-message" {
		if len(split) < 4 {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "Not enough arguments to remove a message",
			})
			return &model.CommandResponse{}, nil
		}
		queue, ok := p.Queues[split[2]]
		if !ok {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   fmt.Sprintf("Unknown queue %s.", split[2]),
			})
			return &model.CommandResponse{}, nil
		}
		idx, err := strconv.ParseUint(split[3], 10, 32)
		if err != nil {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "Invalid position, please see the list-messages command result.",
			})
			return &model.CommandResponse{}, nil
		}
		queue.Messages = append(queue.Messages[:idx], queue.Messages[idx+1:]...)
		nErr := p.SaveQueues()
		if nErr != nil {
			p.API.LogError(nErr.Error())
		}
		_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
			ChannelId: args.ChannelId,
			Message:   "Message removed from the queue",
		})
		return &model.CommandResponse{}, nil
	}

	if split[1] == "insert-message" {
		if len(split) < 5 {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "Not enough arguments to insert a message",
			})
			return &model.CommandResponse{}, nil
		}
		queue, ok := p.Queues[split[2]]
		if !ok {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   fmt.Sprintf("Unknown queue %s.", split[2]),
			})
			return &model.CommandResponse{}, nil
		}
		idx, err := strconv.ParseUint(split[3], 10, 32)
		if err != nil {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "Invalid position, please see the list-messages command result.",
			})
			return &model.CommandResponse{}, nil
		}
		newMessages := []string{}
		for i, message := range queue.Messages {
			if uint64(i) == idx {
				newMessages = append(newMessages, strings.Join(split[4:], " "))
			}
			newMessages = append(newMessages, message)
		}
		queue.Messages = newMessages
		nErr := p.SaveQueues()
		if nErr != nil {
			p.API.LogError(nErr.Error())
		}
		_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
			ChannelId: args.ChannelId,
			Message:   "Message inserted in the queue",
		})
		return &model.CommandResponse{}, nil
	}

	if split[1] == "list-messages" {
		if len(split) < 3 {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "Not enough arguments to list messages",
			})
			return &model.CommandResponse{}, nil
		}
		queue, ok := p.Queues[split[2]]
		if !ok {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   fmt.Sprintf("Unknown queue %s.", split[2]),
			})
			return &model.CommandResponse{}, nil
		}

		listOfMessages := []string{fmt.Sprintf("#### List of messages for the queue %s:", queue.Name)}
		for id, message := range queue.Messages {
			listOfMessages = append(listOfMessages, fmt.Sprintf(" * **%d**: %s", id, message))
		}
		_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
			ChannelId: args.ChannelId,
			Message:   strings.Join(listOfMessages, "\n"),
		})
		return &model.CommandResponse{}, nil
	}
	_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
		ChannelId: args.ChannelId,
		Message:   "Unknown command, please use /" + queueCommand + " help for more information.",
	})
	return &model.CommandResponse{}, nil
}

func (p *Plugin) executeDeferCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	split := strings.Fields(args.Command)
	timeSpec := ""
	if len(split) < 3 {
		if len(split) == 2 && split[1] == "help" {
			return p.executeDeferHelpCommand(c, args)
		}

		return &model.CommandResponse{
			ResponseType: model.COMMAND_RESPONSE_TYPE_EPHEMERAL,
			ChannelId:    args.ChannelId,
			Text:         "Not enough parameters",
		}, nil
	}

	timeSpec = split[1]
	messageStart := strings.Index(args.Command, timeSpec) + len(timeSpec)
	message := args.Command[messageStart:]

	if timeSpec == "online" {
		channel, appErr := p.API.GetChannel(args.ChannelId)
		if appErr != nil {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "Unable to defer the message until the user is online",
			})
			return &model.CommandResponse{}, nil
		}
		if channel.Type != model.CHANNEL_DIRECT {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "Unable to defer the message until the user is online in not DMs channels",
			})
			return &model.CommandResponse{}, nil
		}

		members, appErr := p.API.GetChannelMembers(args.ChannelId, 0, 10)
		if appErr != nil {
			_ = p.API.SendEphemeralPost(args.UserId, &model.Post{
				ChannelId: args.ChannelId,
				Message:   "Unable to defer the message until the user is online",
			})
			p.API.LogError("unable to get channel members of the channel", "err", appErr.Error())
			return &model.CommandResponse{}, nil
		}

		otherUserId := ""
		for _, member := range *members {
			if member.UserId != args.UserId {
				otherUserId = member.UserId
			}
		}

		p.postsWaitingForOnline[otherUserId] = append(p.postsWaitingForOnline[otherUserId], &model.Post{
			UserId:    args.UserId,
			ChannelId: args.ChannelId,
			RootId:    args.RootId,
			ParentId:  args.ParentId,
			Message:   message,
		})
		p.SaveWaitingForOnlinePosts()
		return &model.CommandResponse{
			ResponseType: model.COMMAND_RESPONSE_TYPE_EPHEMERAL,
			ChannelId:    args.ChannelId,
			Text:         "Message deferred",
		}, nil
	}

	duration, err := time.ParseDuration(timeSpec)
	if err != nil {
		return &model.CommandResponse{
				ResponseType: model.COMMAND_RESPONSE_TYPE_EPHEMERAL,
				ChannelId:    args.ChannelId,
				Text:         "Not valid time format",
			}, &model.AppError{
				Message:       "Not valid time format",
				DetailedError: err.Error(),
			}
	}

	deferredPost := model.Post{
		UserId:    args.UserId,
		ChannelId: args.ChannelId,
		RootId:    args.RootId,
		ParentId:  args.ParentId,
		Message:   message,
	}
	p.deferredPosts = append(p.deferredPosts, &DeferredPost{Time: time.Now().Add(duration), Post: &deferredPost})
	p.SaveDeferredPosts()
	model.CreateTask("defer message", func() {
		_, err := p.API.CreatePost(&deferredPost)
		if err != nil {
			p.API.LogError(err.Error())
		}
	}, duration)

	return &model.CommandResponse{
		ResponseType: model.COMMAND_RESPONSE_TYPE_EPHEMERAL,
		ChannelId:    args.ChannelId,
		Text:         "Message deferred",
	}, nil
}

func (p *Plugin) executeDeferHelpCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	helpTitle := `###### Defer Post - Slash Command help
`
	commandHelp := `* |/defer-post [time] [message]| - Send the message after the time has passed
* |/defer-post online [message]| - Send the message when the user is online (only valid for DMs)
* |/defer-post help| - Show this help text

###### Time format:
* The time can be specified in the golang format that you can see [here](https://golang.org/pkg/time/#ParseDuration)`
	text := helpTitle + strings.Replace(commandHelp, "|", "`", -1)
	post := &model.Post{
		ChannelId: args.ChannelId,
		Message:   text,
	}
	_ = p.API.SendEphemeralPost(args.UserId, post)

	return &model.CommandResponse{}, nil
}

func (p *Plugin) executeQueueHelpCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	helpTitle := `###### Messages Queue - Slash Command help
`
	commandHelp := `* |/messages-queue create <name> <schedule>| - Create a queue for the current channel (see the Schedule format help at the bottom)
* |/messages-queue list| - List the queues for this channel
* |/messages-queue delete <queue-name>| - Delete a queue.
* |/messages-queue add-message <queue-name> <message>| - Add a new message to the queue
* |/messages-queue list-messages <queue-name>| - Add a new message the the queue
* |/messages-queue remove-message <queue-name> <position>| - Remove a message from the queue in the specified position
* |/messages-queue insert-message <queue-name> <position> <message>| - Add a new message to the queue in the specified position
* |/messages-queue help| - Show this help text

###### Schedule format:
* The schedule format used is the cron expresion format, you can see more information [here](https://en.wikipedia.org/wiki/Cron)

###### Queue names:
* The queue names must can be anything without spaces in it`
	text := helpTitle + strings.Replace(commandHelp, "|", "`", -1)
	post := &model.Post{
		ChannelId: args.ChannelId,
		Message:   text,
	}
	_ = p.API.SendEphemeralPost(args.UserId, post)

	return &model.CommandResponse{}, nil
}

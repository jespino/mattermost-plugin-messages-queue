# Mattermost Message Queue

This plugin adds async messages delivery based on time or online status of the users throuh `/defer-post` and `/messages-queue` slash commands.

## `/defer-post`

The `/defer-post` commands that allows to defer the delivery of a message for certain amount of time, or until the other user is online (in DMs).

### Available commands

  * `/defer-post [time] [message]` - Send the message after the time has passed
  * `/defer-post online [message]` - Send the message when the user is online (only valid for DMs)

### Defer time format

The time can be specified in the golang format that you can see [here](https://golang.org/pkg/time/#ParseDuration).

### Examples

  * In a DM you can run `/defer-post online please take a lot to this ticket
    #123`. That will send the message to the user whenever the user is online
    again, so you don't have to worry about annoy him while he is offline (no
    message, and no notifications).
  * In a any channel you can run `/defer-post 2h Starting the deployment`. This
    will schedule the message to be send in 2 hours.

## `/messages-queue`

The `/messages-queue` commands allows you to create and maintain messages
queues that are send base on a schedule.

You can list, add and delete queues, and the messages in it. Every time the
scheduled message queue is triggered, it sends one message from the queue. So
if you have a queue with 20 messages, and your schedule is to run it every day,
is going to take 20 days to send all the messages.

When the queue is empty, no messsage is sent.

### Available commands

  * `/messages-queue create <name> <schedule>` - Create a queue for the current channel (see the Schedule format help at the bottom)
  * `/messages-queue list` - List the queues for this channel
  * `/messages-queue delete <queue-name>` - Delete a queue.
  * `/messages-queue add-message <queue-name> <message>` - Add a new message to the queue
  * `/messages-queue list-messages <queue-name>` - Add a new message the the queue
  * `/messages-queue remove-message <queue-name> <position>` - Remove a message from the queue in the specified position
  * `/messages-queue insert-message <queue-name> <position> <message>` - Add a new message to the queue in the speicified position

### Schedule format

The schedule format used is the cron expresion format, you can see more information [here](https://en.wikipedia.org/wiki/Cron).

### Example

  * If you want to prepare a set of tips to send them from monday to friday at 10 am to your users you can run:
    * `/message-queue create tips 0 10 * * 1-5`
    * `/message-add-message tips remember to take some breaks during the day.`
    * `/message-add-message tips remember to send your standup summary to the standup channel.`
    * `/message-add-message tips Do you know you can expense books and learning material? Please do it!`

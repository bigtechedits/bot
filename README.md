bot
===

This repository hosts the code for the [Twitter](https://twitter.com) bot [bigtechedits](https://twitter.com/bigtechedits).
It subscribes to changes that are made on [Wikipedia](https://wikipedia.org). From this stream it looks for changes that are made anonymously. It tweets about these anonymous change, if the author used a big tech providers cloud.

## how to run it
To be able to post tweets some environment variables are required.
`TWITTER_CLIENT_ID`, `TWITTER_CLIENT_SECRET` and  `TWITTER_REDIRECT_URI` are the appropriate elements as configured in the [Twitter Developer Portal](https://developer.twitter.com/en/portal/dashboard) for the project. For the [OAuth 2.0](https://en.wikipedia.org/wiki/OAuth) part of the authentication process files are used. Once this bot is started with the environment variables set `/opt/bigtechedits/AuthCodeURL` will hold the authentication code URL. Then `/opt/bigtechedits/AuthCode` is checked repeatedly for the authentication code.

## kudos
This bot is inspired by [@bundesedit](https://twitter.com/bundesedit), [@congressedits](https://twitter.com/congressedits) and similar bots.

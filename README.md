# Skipfilter

This package provides a data structure that combines a skip list with a roaring bitmap cache.

[![Go Reference](https://pkg.go.dev/badge/github.com/dunglas/skipfilter.svg)](https://pkg.go.dev/github.com/kevburnsjr/skipfilter)
[![Go Report Card](https://goreportcard.com/badge/github.com/dunglas/skipfilter?3)](https://goreportcard.com/report/github.com/kevburnsjr/skipfilter)

> [!NOTE]
>
> This a maintained and improved fork of [github.com/kevburnsjr/skipfilter](https://github.com/kevburnsjr/skipfilter)

This library was created to efficiently filter a multi-topic message input stream against a set of subscribers,
each having a list of topic subscriptions expressed as regular expressions. Ideally, each subscriber should test
each topic at most once to determine whether it wants to receive messages from the topic.

In this case, the skip list provides an efficient discontinuous slice of subscribers and the roaring bitmap for each
topic provides an efficient ordered discontinuous set of all subscribers that have indicated that they wish to
receive messages on the topic.

Filter bitmaps are stored in a cache of variable size (default to unlimited).

This package is thread-safe.

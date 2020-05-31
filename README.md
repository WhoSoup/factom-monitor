# Factom Monitor

A library to listen to height events coming from a factom node, either minute events or block events.

## Motivation

I found myself writing similar code over and over again: a polling loop that reacts when the height or minute event in factomd moves. This library aims to be a lightweight implementation of that polling cycle so it can be imported in other projects.

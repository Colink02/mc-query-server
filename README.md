# MC Standalone Query Server
This is a standalone query server for Minecraft with the intention of seperating the query server from the game server.
Initially to be used with HAProxy to allow for a single query server to be used for multiple game servers.

The configuration file `config.jsonc` contains a configuration for a static query.

This project was initially built for The Enfinium Network to send all query requests to a single server instead of sending
the request to velocity proxy to be handled there and take in values from other data sources for proper querying of the 
player count across all proxies.

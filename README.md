# Arkstorm
Youtube video auto creation and upload tool.

# Summary

- BUGBUG: Write purpose of app
- BUGBUG: Write usecase of app

# Dependencies
## Build Environment

- Binary
  - Golang 1.19
  - Build Essentials (Linux)
- Docker
  - docker

## Runtime Environment

- [ffmpeg installed and in path](https://ffmpeg.org/download.html)
- docker (for running container locally)

## Runtime Parameters
### App Configuration
- Toml Config file: ./arkstorm [config file path]. 

Note: Passed as single argument to app, otherwise defaults to $CWD\config.toml.<br>
Note: See example-configs folder for examples

### Weatherbell Access  
- Username env variable: **WEATHERBELL_USERNAME=[username]**
- Password env variable: **WEATHERBELL_PASSWORD=[password]**
- Session ID env variable: **WEATHERBELL_SESSIONID=[sessionid]**

Note: WEATHERBELL_SESSIONID is for development. It stops app from requesting new session ID everytime. It also invalidates WEATHERBELL_USERNAME and WEATHERBELL_PASSWORD.

### Youtube Access
- Secrets file: **$CWD\client_secret.json**
- Token file: **$CWD\client_token.json**

Note: See [youtube-token-generator README.md](youtube-token-generator/README.md) for instructions on how to create these files.

# Building and Running
## Building

```
# build local binary executable
$make build

# build docker image
$make docker
```
## Running

```
make run config=[toml config file path]
```

# Additional Info

## Important Notes

- Originally developed on Linux Mint 21.1

## Limitations

- None!, it's a beast

## Known Issues 

- None!, it's perfect

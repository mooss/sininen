# Sininen

Sininen's goal is to provide tools to perform natural language queries on text data.
Right now it is focused on searching though subtitles extracted from YouTube channels.

## Usage

### Download subtitles from a channel

The channel name is needed.
To find it from a video like https://www.youtube.com/watch?v=aq4G-7v-_xI, click on the channel name (here Historia Civilis), landing on the page https://www.youtube.com/channel/UCv_vLHiWVBh_FR9vbeuiY-A.
Then click on the HOME tab, this changes the URL to https://www.youtube.com/c/HistoriaCivilis/featured.
The channel name is the string after `/c/`, here HistoriaCivilis.

Download the subtitles for HistoriaCivilis with:
```sh
./download-channel-subtitles.sh HistoriaCivilis
```

### Build YouTube CLI

```sh
go get
go build cli/search-yt.go
```

### Search through channel subtitles

```sh
./search-yt HistoriaCivilis "Crossing the Rubicon"
```

# TRY ME

This service has two components:
* redis 7.0.9 server, listening on port 6379
* the state engine itself
    * this service will receive logs from clients, currently through json payload over http.
        * Endpoint: "/discrete"
    * visualization is provided by the state engine. 
        * Endpoint on "/"

# How to launch

## Step 1: Start the REDIS server

Requirements: docker and docker-compose

This will only launch the redis server. 
```sh
docker-compose up
```

## Step 2: Start the Backend Server

> Requirement: go running environment


```sh
make run-local # | grep -v discrete
```

This will execute the service. It is listening on port http 8080


Alternatively, you can use the following if you want the backend server to be reachable from any IP. Note it's a bad idea to put it straight on the Internet.
```sh
make run-open # | grep -v discrete
```

The endpoints are:
* / -> Matrix Style Visualization
* /discrete -> (POST) end point receiving events

## Step 3: Sending events to the Backend Server

The backend server expects to receive log events on http port 8080 (default) once at the time through a post request.

The expected fields are the following
* "org": "string" -> organization name, this is used to split contexts
* "source": "producer" -> this is used to identify the service where events comes from
* "pop": "normal users" -> used to identify the population name. This population is expected to have a similar behavior profile
* "id": "unique identifier" -> should identify a single individual or agent
* "action": "unique action" -> string desribing the discrete action performed by the individual

```sh
curl -X POST -H "Content-Type: application/json" -d '{"org": "acme", "source": "webserver", "pop": "auth_getpost", "id": "individual1", "action": "GET /user/login"}' http://127.0.0.1:8080/discrete
```

The backend server will return a response that looks like this:

```json
{"runtime":1,"score":0,"count":1,"zscore":-1.158,"source":"webserver","pop":"auth_getpost","id":"individual1","action":"GET /user/login"}
```

* "runtime": integer -> number of milliseconds required to handle the query
* "score": float -> bits information value for the current id.
* "count": integer -> number of events received for the current id
* "zscore": float -> number of standard deviation away from the given population. The greater the value, the further the id is away from the population's normal profile.

* "source": string -> echo back of the provided source
* "pop": string -> echo back of the population name provided
* "id": string -> echo back of the id provided
* "action": string -> echo back of the action provided.

Any action from the querying client should be conditioned on `zscore`.
* zscore < 0: Individuals with values under 0 means that they are under the average. This should mean that their behavior is ok
* zscore >= 1: top 84% of the individuals. this should still be ok, but should represent power users
* zscore >= 2: top 95% of the individuals. they can still be ok, but it's less likely
* zscore >= 3: top 99.9% of the individuals. these start to be significant outliers
* zscore >= 4: this should only happen when an individual's behavior is way outside of the normal model.

## Example of usage cases

* "org" and "source" should be your organization and service generating the data.

* "pop" and "id" can take many shapes. for example:
    * pop: all authenticated users, id: the identifier of the user
    * pop: all unauthenticated users, id: the ip address, user agent, or both
    * pop: the name of a microservice, id: an instance that microservice. example: CI/CD workers.
    * pop: a shared user id, id: the IP address used by each user

Notice that I try to group users in a population that I expect to behave in a similar way. Random individuals in a unspecified population will result in inconsistent behavior profiles, making the system useless.

## Maintenance of the system

* The state engine is self normalizing itself regarding the counters.
* Individuals are not purged from the system after a long inactivity. This has the advantage of leaving samples of previous sessions, which is used to calculate the baseline.

This implies that the Redis database must have sufficiently enough memory to keep track of all the counters for all the org/source/population/individuals processed by the state engine.

The service isn't limited to a single context. It will split each of them internally based on the parameters provided when a client request is made.


## Training period

A training period isn't really necessary. The way the calculations are made, the results should rebalance themselves as new values are observed. 

When a new and unexpected behavior is expected, the system will rebalance itself. However, if a single individual skew all the values out of proportion, I would expect that all users will start to be considered as outliers. That should be detectable before it happens.


## Feeding a log file to the Backend server

> Requirement: python3, jq (optional)

I added an example python3 script can be used to feed a linefeed delimited JSON file to the backend server.

* `feed_lib_processor.py`: Configuration file. It is a library because it can be used to format logs before feeding them to the backend server. **This file will require modifications**
* `feed_sample_threaded.py`: main code

### Modify the `feed_lib_processor.py` file

#### General variables
```
log_file = "webserver_logfile.json" # enter delimited json file
state_file = "feed_state_01.txt"
endpoint_url = "http://localhost:8080/discrete"
```

* `log_file` should be updated
* `state_file` is used to keep track of the progress. if the feeding script is stopped, it will restart from the line number in this file
* `endpoint_url`: modify to fit the IP and port of the backend server, if it is not running locally

#### Line processing function `def process_event(e):`

This function will take the dict variable provided by the `producer` function, and give an opportunity to modify it as needed to fit any transformation needs you might have.

It will then return the needed parameters to be sent to the backend server.

> Note: because the code is using threading, errors in the code will not always show up on the console screen. If the script starts but does nothing (queues remains empty), check that the `process_event` function doesn't have any error. 

### Executing the feeding script

This script will only display responses from the server when the `score` value is greater than the previously observed one. The goal being to reduce the output while testing the server.

A user who's behavior is leaning back toward "normal" will not be displayed again by this script.

Here is an example command that can be used to start the feeding.
```sh
time python3 feed_sample_threaded.py |tee debug.json |jq -c '.|select(.zscore > 4)'
```

* `time`: (optional) useful to know how long it takes to feed the log file
* `tee debug.json`: will dump the output of the script to `debug.json` then display it.
* `jq -c '.|select(.zscore > 4)'`: uses `jq` to filter the output to only display `zscore` over 4


### About Performance

Depending on the system you are running this on, performance will vary. Using a sample log file containing 1M entries, feeding them to a single backend server and redis database, using the `feed_sample_threaded.py` script above:

* Locally on a Mac M1: 1M events in ~3462 seconds. ~288 events per second.
* Locally on an Intel i7-7700HQ (4 cores HT): 1M events in ~2013 seconds. ~497 events per second.

Running this system over a network where the redis database is on a different host do increase the average runtime significantly.

## Visualization

With the backend server running, connect with a browser on "http://localhost:8080/" (or ip of the server).

Chome normally works well. Firefox is normally ok too, but some plugins might disrupt the CONNECT command.

At the top of the window, you will see three sliders:
* `zscore filter`: default to -3, and will adjust to match any new lower zscore if the slider was at the lowest value already.
* `red filter`: determine at how many standard deviations the characters will turn red. default to 3 and higher, maximum 5.
* `character size`: font size of the characters, minimum 6, maximum 48. The visualization will wrap arround if there are more unique IDs than columns (window width / character size).

An extra line is left at the bottom so the sliders can be hidden by scrolling the page down.

Positions of characters and window size are recalculated on load (or reload) only.


### Debugging notes

* The `feed_sample_threaded.py` script is using threads to send events to backend server as much as possible. 
* If it ends normally, it will exit gracefully.
* ctrl-c should stop it gracefully as well, if there is no error in the code.
* However it doesn't handle edge conditions well. If the server is not answering, or if there is an error in the code of the processing library, it will just hang
* hit ctrl-z to break out of it.
* this is provided as an example, that should not be used in prod.
* If you want to re-feed a log file from zero, it is preferable to:
    * flush the redis server (see below)
    * and delete the state file (ex.: `feed_state_01.txt`)


## Clearing the redis server

The service is built to injest a constant stream of events. Replaying events will debalance the probabilities.

```sh
~$ docker ps
CONTAINER ID   IMAGE                COMMAND                  CREATED       STATUS      PORTS                                       NAMES
fb9055d0f232   redis:7.0.9-alpine   "docker-entrypoint.s…"   3 weeks ago   Up 7 days   0.0.0.0:6379->6379/tcp, :::6379->6379/tcp   looking-binomial_redis_1

$ docker exec -i fb9055d0f232 redis-cli flushall
OK
```


# FIXME
- docker-compose should start the server as well, but I have a race condition and the service starts faster than redis
- the code isn't handling connection failures well.
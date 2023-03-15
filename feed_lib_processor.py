# This file is meant to be heavily updated to match the context
# of the service you entend to consume logs from
#
# the process event function will take the original log file
# and format it in an object matching the expected format from the /discrete endpoint
# on the server side.
# 
# The final format rquires the returned fields
# org is if your need to deal with multiple companies and need to split context
# source: is the service name where the logs comes from
# pop: is the population name. a population is expected to behave similarly
# id: is the unique identifier for the subject to analyze
# action: must be a discrete action. 
#   too few and it will be hard to create a good model of what is being done
#   too many and everything will be unlikely
#   try and see, but I think that anything 10 and 200 should be a good target
# 
# Notice that there is no date field.
# the server is ment to work on a live log stream, or with replays of logs
# as long as the logs are fed in a chronological order, results should be consistent

import math

log_file = "webserver_logfile.json" # enter delimited json file

state_file = "feed_state_01.txt"
endpoint_url = "http://localhost:8080/discrete"

def process_event(e): # MODIFY ME TO FIT YOUR SITUATION

    # custom modifications to filter junk
    if e['path'].endswith("/"):
        return False

    if e['path'].startswith("/cases/"):
        e['path'] = "/cases"

    # transform continuous size values into groups
    if e['response_size'] != "0":
        e['size'] = 10 ** math.ceil(math.log10(int(e['response_size'])))
    else:
        e['size'] = 0

    # glue all the fields together to make up a distinct action
    action = [str(e[x]) for x in ["method", "path", "status", "size"]]

    # return the result to the main code to be submitted.
    return {
        "org": "acme",
        "source": "webserver",
        "pop": e["service"],
        "id": " ".join([e['host'], e['subject'], e["user_agent"]]),
        "action": " ".join(action)
    }


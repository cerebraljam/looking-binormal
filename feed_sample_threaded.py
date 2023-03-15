import json
import requests
import os
import sys
import signal
import time

import concurrent.futures
import logging
import queue
import threading

from feed_lib_processor import endpoint_url, log_file, state_file, process_event

last_line = 0
if os.path.exists(state_file):
    s = open(state_file, "r")
    line = s.readline()
    if line != "":
        last_line = int(line)
    s.close()


class escalation_state:
    def __init__(self):
        self.data = {}
    
    def update(self, key, value):            
        if key not in self.data.keys():
            self.data[key] = value
            return True
        elif value > self.data[key]:
            self.data[key] = value
            return True

        return False


escalation = {}

class Pipeline(queue.Queue):
    def __init__(self):
        super().__init__(maxsize=10)

    def get_message(self, name):
        logging.debug("%s:about to get from queue", name)
        value = self.get()
        logging.debug("%s:got %d from queue", name, value)
        return value


    def set_message(self, value, name):
        logging.debug("%s:about to add %d to queue", name, value)
        self.put(value)
        logging.debug("%s:added %d to queue", name, value)


def displayer(queue, event):
    history = escalation_state()

    logging.info("Displayer: Starting with a queue of  %d events", queue.qsize())

    while not event.is_set() or not queue.empty():
        res = queue.get()

        mention = history.update(res['id'], res['score'])
        if mention:
            print(json.dumps(res))
    
    logging.info("Displayer received event. Exiting. queue size=%d", queue.qsize())
    return

def consumer(queue, event, displayqueue):
    logging.info("Consumer: Starting with a queue of  %d events", queue.qsize())

    while not event.is_set() or not queue.empty():
        payload = queue.get()

        r = requests.post(url = endpoint_url, json = payload)
        res = json.loads(r.text)

        if r.status_code != 200:
            logging.info("Consumer: Server returned status code %d. Exiting", r.status_code)
            event.set()
            return

        displayqueue.put(res)

    logging.info("Consumer received event. Exiting. queue size=%d", queue.qsize())
    return

def producer(queue, event, last_line):
    linecounter = 0

    logging.info("Producer: starting at line %d", last_line)
    logging.info("* Opening file: %s", log_file)
    if not os.path.exists(log_file):
        logging.info("* log file does not exists: %s", log_file)
        event.set()
        return

    with open(log_file, 'r') as f:
        for line in f:
            if event.is_set():
                logging.info("Producer received event. Exiting")
                break

            linecounter+=1
            if linecounter < last_line:
                continue

            payload = process_event(json.loads(line))
            if payload != False:
                queue.put(payload)
            
            if linecounter % 100 == 0:
                with open(state_file, "w") as s:
                    s.write(str(linecounter))

    with open(state_file, "w") as s:
        s.write(str(linecounter))

    event.set()
    logging.info("Producer is done. Exiting")
    return


if __name__ == "__main__":
    format = "%(asctime)s: %(message)s"
    logging.basicConfig(format=format, level=logging.INFO, datefmt="%H:%M:%S")
    # logging.getLogger().setLevel(logging.DEBUG)

    workers = os.cpu_count()
    logging.info("Main starting with %d consumers", workers)

    pipeline = queue.Queue(maxsize=10)
    displaypipeline = queue.Queue(maxsize=10*workers)
    event = threading.Event()

    def interrupt_handler(signum, frame):
        event.set()
        logging.info("Main interrupted: asking workers to stop. waiting 2 seconds then will exit")
        time.sleep(2)
        sys.exit(1)
    
    signal.signal(signal.SIGINT, interrupt_handler)
    

    with concurrent.futures.ThreadPoolExecutor(max_workers=workers+2) as executor:
        executor.submit(producer, pipeline, event, last_line)
        executor.submit(displayer, displaypipeline, event)

        for i in range(workers):
            executor.submit(consumer, pipeline, event, displaypipeline)


# time python3 feed_sample_threaded.py |tee debug.json |jq -c '.|select(.zscore > 4)'

# monitoring progression in the logfile
# watch wc -l debug.json

# monitoring rate of output
# tail -f debug.json | { count=0; old=$(date +%s); while read line; do ((count++)); s=$(date +%s); if [ "$s" -ne "$old" ]; then echo "$count lines per second"; count=0; old=$s; fi; done; }

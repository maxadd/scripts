#!/usr/local/python3/bin/python3

import os
import re
import sys
import etcd
import time
import queue
import logging
import platform
import threading
import subprocess
from git import Repo

instances = (("10.0.0.1", 2379), ("10.0.0.2", 2379), ("10.0.0.3", 2379))
default_zone = "pro"
log_file = "/var/log/autobind.log"
bind_conf = "/etc/named.conf"
bind_conf_dir = "/etc/named"
zone_dir = "/var/named"
pid_file = "/var/tmp/autoBind.pid"
include_content = 'include "/etc/named/{}.conf";\n'
bind_conf_template = """zone "{}" IN {{
    type master;
    file "{}";
    allow-update {{ none; }};
}};
"""

forward_zone_head = """$TTL 86400
@   IN  SOA  {node}    root.{zone}. (
            2014112511  ;Serial
            3600        ;Refresh
            1800        ;Retry
            604800      ;Expire
            86400       ;Minimum TTL
)
@ IN NS ns1.{zone}.
@ IN NS ns2.{zone}.
@ IN NS ns3.{zone}.
@ IN NS ns4.{zone}.
ns1 IN A 10.0.0.1
ns2 IN A 10.0.0.2
ns3 IN A 10.0.0.3
ns4 IN A 10.0.0.4
"""

reverse_zone_head = """$TTL 86400
@   IN  SOA  {node}    root.{zone}. (
            2014112511  ;Serial
            3600        ;Refresh
            1800        ;Retry
            604800      ;Expire
            86400       ;Minimum TTL
)
@ IN NS ns1.{zone}.
@ IN NS ns2.{zone}.
@ IN NS ns3.{zone}.
@ IN NS ns4.{zone}.
1 IN PTR ns1.{zone}.
2 IN PTR ns2.{zone}.
3 IN PTR ns3.{zone}.
4 IN PTR ns4.{zone}.
"""

client = etcd.Client(instances, allow_reconnect=True)
FQDN = platform.node()
logging.basicConfig(filename=log_file, level=logging.INFO, format="%(asctime)s %(levelname)s %(lineno)d %(message)s")
q = queue.Queue()
reload_named_q = queue.Queue()
git = Repo(zone_dir).git
patten = re.compile(r'\s')
if os.path.exists(pid_file):
    print("program is running")
    sys.exit(1)
with open(pid_file, 'w') as f:
    f.write(str(os.getpid()) + "\n")


class Return(Exception):
    pass


class Exit(Exception):
    pass


class Record:
    def __init__(self, event):
        self.event = event
        self.zones = []
        try:
            _, _, self.domain, self.hostname, self.ip = event.key.split("/")
            self.ip_verify(self.ip)
            tmp = self.hostname.split("." + self.domain)
            if len(tmp) != 2 or tmp[-1] != "":
                logging.error("unexpected hostname:", self.hostname)
                raise Return
            # _, _, _, _, _ = self.hostname.split(".")
        except ValueError:
            logging.error("unexpected key:",  event.key)
            raise Return

    @staticmethod
    def ip_verify(ip):
        tmp = ip.split(".")
        flag = False

        for i in tmp:
            if not i.isdigit():
                flag = True

        if len(tmp) != 4 or flag:
            logging.error(f'unexpected ip:, {ip}')
            raise Return

    @staticmethod
    def update_file(file, prefix, content, element):
        with open(file, 'r+') as f:
            offset = 0
            for i in f:
                if re.match(prefix + r'\s', i):
                    if patten.split(i.rstrip())[-1] == element:
                        logging.warning(f'the record {prefix} already exists in file {file}, '
                                        f'{i.strip()} -> {content}')
                        break
                offset += len(i)
            else:
                f.write(content + '\n')
                return
            lave_content = ""
            for i in f:
                lave_content += i
            f.seek(offset)
            f.truncate(offset)
            f.write(lave_content)
            f.write(content + '\n')

    def forward_zone_update(self, forward_zone_file, forward_conf_file):
        self.append_conf_file(forward_conf_file, self.domain)
        # hostname = self.hostname.rstrip("." + self.domain)
        hostname = self.hostname[:-len("." + self.domain)]
        content = f'{hostname} IN A {self.ip}'
        try:
            self.update_file(forward_zone_file, hostname, content, self.ip)
        except FileNotFoundError:
            f = self.create_forward_zone_file(forward_zone_file)
            f.write(content + "\n")
            f.close()

    def reverse_zone_update(self, reverse_zone_file, reverse_conf_file, network):
        self.append_conf_file(reverse_conf_file, network)
        ip = self.ip.split(".")[-1]
        content = f'{ip} IN PTR {self.hostname}'
        try:
            self.update_file(reverse_zone_file, ip, content, self.hostname)
        except FileNotFoundError:
            f = self.create_reverse_zone_file(reverse_zone_file)
            f.write(content + "\n")
            f.close()

    def set(self):
        forward_zone_file = os.path.join(zone_dir, self.domain + ".zone")
        forward_conf_file = os.path.join(bind_conf_dir, self.domain + ".conf")
        tmp = self.ip.split(".")
        _network = tmp[:3]
        reverse_zone_file = os.path.join(zone_dir, '.'.join(_network) + ".zone")
        network = ".".join(_network)
        network_reverse = tmp[:3]
        network_reverse.reverse()
        reverse_zone = '.'.join(network_reverse) + ".in-addr.arpa"
        reverse_conf_file = os.path.join(bind_conf_dir, network + ".conf")
        self.zones.extend([(self.domain, forward_zone_file), (reverse_zone, reverse_zone_file)])
        if not os.path.isfile(forward_conf_file):
            self.forward_create(forward_zone_file, forward_conf_file)
        else:
            self.forward_zone_update(forward_zone_file, forward_conf_file)
        if not os.path.isfile(reverse_conf_file):
            self.reverse_create(reverse_zone_file, reverse_conf_file, network, reverse_zone)
        else:
            self.reverse_zone_update(reverse_zone_file, reverse_conf_file, network)

    @staticmethod
    def append_conf_file(file, content):
        with open(bind_conf, 'r+') as conf:
            for i in conf:
                if i.startswith("include"):
                    if i.split()[1].rstrip(';').strip('"').strip("'") == file:
                        break
            else:
                conf.write(include_content.format(content))

    def forward_create(self, forward_zone_file, forward_conf_file):
        with open(forward_conf_file, 'w') as f:
            f.write(bind_conf_template.format(self.domain, self.domain + ".zone"))
        self.forward_zone_update(forward_zone_file, forward_conf_file)

    def reverse_create(self, reverse_zone_file, reverse_conf_file, network, reverse_zone):
        with open(reverse_conf_file, 'w') as f:
            f.write(bind_conf_template.format(reverse_zone, network + ".zone"))
        self.reverse_zone_update(reverse_zone_file, reverse_conf_file, network)

    @staticmethod
    def create_reverse_zone_file(file):
        f = open(file, 'w')
        f.write(reverse_zone_head.format(node=FQDN, zone=default_zone))
        return f

    @staticmethod
    def create_forward_zone_file(file):
        f = open(file, 'w')
        f.write(forward_zone_head.format(node=FQDN, zone=default_zone))
        return f

    @staticmethod
    def delete_record(file, prefix, element):
        with open(file, 'r+') as f:
            offset = 0
            for i in f:
                if re.match(prefix + r'\s', i):
                    if patten.split(i.rstrip())[-1] == element:
                        break
                offset += len(i)
            else:
                logging.error(f'when deleting, {prefix} does not exist in the {file} file')
                raise Return
            lave_content = ""
            for i in f:
                lave_content += i
            f.seek(offset)
            f.truncate(offset)
            f.write(lave_content)
            logging.warning(f'delete record {prefix} success!')

    def delete(self):
        forward_zone_file = os.path.join(zone_dir, self.domain + ".zone")
        hostname_prefix = self.hostname[:len(self.hostname) - len("." + self.domain)]
        self.delete_record(forward_zone_file, hostname_prefix, self.ip)
        ip_split = self.ip.split(".")
        reverse_zone_file = os.path.join(zone_dir, '.'.join(ip_split[:3]) + ".zone")
        network_reverse = ip_split[:3]
        network_reverse.reverse()
        reverse_zone = '.'.join(network_reverse) + ".in-addr.arpa"
        self.zones.extend([(self.domain, forward_zone_file), (reverse_zone, reverse_zone_file)])
        self.delete_record(reverse_zone_file, ip_split[-1], self.hostname)


def reload_named():
    try:
        subprocess.check_output("named-checkconf")
        # for i in zones:
        #     subprocess.check_output(["named-checkzone", i[0], i[1]])
        subprocess.check_call("systemctl reload named", shell=True)
        logging.warning("restart named succeeded!")
    except subprocess.CalledProcessError as e:
        logging.exception("conf check failed")
        raise Exit("conf check failed", e)


def wait_reload_named():
    while True:
        reload_named_q.get()
        logging.info("restart named after 5 seconds")
        time.sleep(5)
        reload_named()
        try:
            while True:
                reload_named_q.get(block=False)
        except queue.Empty:
            pass


def git_opt(zones):
    if len(zones) == 0:
        return
    for i in zones:
        git.add(i[1])
    git.commit("-m", time.strftime('%Y-%m-%d %H:%M:%S', time.localtime()))


def get_event_obj():
    while True:
        logging.info("waiting event is triggered")
        event = q.get()
        logging.info(f'get event {event.key} from queue')
        try:
            obj = Record(event)
            # print(event.action)
            getattr(obj, event.action)()
            # print(obj.zones)
            # reload_named(obj.zones)
            # git_opt(obj.zones)
        except Return:
            pass
        # except git.exc.GitCommandError:
        #     logging.exception("")
        except (Exception, Exit) as e:
            print(e)
            logging.exception("")
            os.remove(pid_file)
            os.kill(os.getpid(), 15)


threading.Thread(target=get_event_obj, daemon=True).start()
threading.Thread(target=wait_reload_named, daemon=True).start()

while True:
    try:
        # /dns/idc01/ops.xxx.com/10.0.0.1
        event = client.watch("/dns/", recursive=True, timeout=1800)
        # logging.info("get event {}, put it to queue".format(event.key))
        q.put(event)
        reload_named_q.put(1)
    except etcd.EtcdWatchTimedOut:
        pass

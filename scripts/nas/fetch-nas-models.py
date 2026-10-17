#!/usr/bin/env python3
"""把 NAS 上的模型配置导成可回灌本机 SQLite 的 JSON 数据包。

用法：
    fetch-nas-models.py --section models --host 192.168.1.56 --port 3050 \
        --username xupc --password … --channel CHANNEL_000001 > models.json

设计要点：
  * 只读：全部走 GET 接口，不改动 NAS 任何数据。
  * 字段名以 NAS 的 JSON（camelCase）为准，落库时再映射回 SQLite 的 snake_case，
    避免脚本里散落两套命名。
  * 输出带 schema 版本与抓取时间，回灌脚本据此校验，模式漂移时直接报错而不是写坏数据。

退出码：0 成功；2 参数错误；3 登录失败；4 接口返回非 0。
"""

import argparse
import json
import ssl
import sys
import urllib.error
import urllib.parse
import urllib.request

SCHEMA_VERSION = 1


class ApiError(RuntimeError):
    pass


class NasClient:
    """最小化 NAS 后端客户端：登录拿 Cookie，只发 GET。"""

    def __init__(self, host, port, username, password, timeout=30):
        self.base = "https://%s:%s" % (host, port)
        self.username = username
        self.password = password
        self.timeout = timeout
        self.cookie = ""
        # NAS 用自签证书，属于局域网内的已知主机，这里只跳过校验不做告警抑制。
        self.ctx = ssl.create_default_context()
        self.ctx.check_hostname = False
        self.ctx.verify_mode = ssl.CERT_NONE

    def _request(self, path, data=None, headers=None):
        url = self.base + path
        merged = {"Accept": "application/json"}
        if self.cookie:
            merged["Cookie"] = self.cookie
        if headers:
            merged.update(headers)
        body = None
        if data is not None:
            body = json.dumps(data).encode("utf-8")
            merged["Content-Type"] = "application/json"
        req = urllib.request.Request(url, data=body, headers=merged)
        try:
            resp = urllib.request.urlopen(req, context=self.ctx, timeout=self.timeout)
        except urllib.error.HTTPError as exc:
            detail = exc.read().decode("utf-8", "replace")[:300]
            raise ApiError("%s %s -> HTTP %s %s" % (req.get_method(), path, exc.code, detail))
        except urllib.error.URLError as exc:
            raise ApiError("%s %s -> 连接失败：%s" % (req.get_method(), path, exc.reason))
        payload = json.loads(resp.read().decode("utf-8"))
        if payload.get("code") != 0:
            raise ApiError("%s %s -> code=%s msg=%s" % (
                req.get_method(), path, payload.get("code"), payload.get("msg")))
        return payload.get("data"), resp.headers

    def login(self):
        _, headers = self._request(
            "/api/auth/login",
            data={"username": self.username, "password": self.password},
        )
        raw = headers.get("set-cookie") or ""
        cookie = raw.split(";", 1)[0]
        if not cookie or "=" not in cookie:
            raise ApiError("登录成功但没有拿到会话 Cookie")
        self.cookie = cookie

    def get(self, path):
        data, _ = self._request(path)
        return data


def fetch_channels(client, scope):
    query = urllib.parse.urlencode({"scope": scope}) if scope else ""
    path = "/api/admin/channels" + ("?" + query if query else "")
    data = client.get(path)
    return data.get("channels") or []


def fetch_channel_models(client, channel_id):
    data = client.get("/api/admin/channels/%s/models" % urllib.parse.quote(channel_id))
    return data.get("models") or []


def main():
    parser = argparse.ArgumentParser(description="导出 NAS 上的模型配置为 JSON")
    parser.add_argument("--host", required=True)
    parser.add_argument("--port", default="3050")
    parser.add_argument("--username", required=True)
    parser.add_argument("--password", required=True)
    parser.add_argument("--scope", default="system", help="渠道作用域：system / user / all")
    parser.add_argument("--channel", default="", help="只导某个渠道 ID，默认全部")
    parser.add_argument("--section", default="models", choices=["models", "all"],
                        help="models=只导渠道模型；all=渠道模型 + 渠道本身")
    args = parser.parse_args()

    client = NasClient(args.host, args.port, args.username, args.password)
    try:
        client.login()
        channels = fetch_channels(client, args.scope)
        if args.channel:
            channels = [c for c in channels if c.get("id") == args.channel]
            if not channels:
                raise ApiError("NAS 上找不到渠道 %s" % args.channel)

        models = []
        for channel in channels:
            for model in fetch_channel_models(client, channel["id"]):
                model = dict(model)
                model["_channelName"] = channel.get("name", "")
                models.append(model)
    except ApiError as exc:
        sys.stderr.write("错误：%s\n" % exc)
        return 4

    payload = {
        "schemaVersion": SCHEMA_VERSION,
        "source": {"host": args.host, "port": args.port, "scope": args.scope},
        "channels": [
            {
                "id": c.get("id"),
                "name": c.get("name", ""),
                "displayName": c.get("displayName", ""),
                "scope": c.get("scope", ""),
                "apiFormat": c.get("apiFormat", ""),
                "baseUrl": c.get("baseUrl", ""),
            }
            for c in channels
        ] if args.section == "all" else [],
        "models": models,
    }
    json.dump(payload, sys.stdout, ensure_ascii=False, indent=2)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

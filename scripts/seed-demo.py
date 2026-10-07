#!/usr/bin/env python3
"""Load Pandora demo data through the HTTP API into an empty database.

Intended flow: reset the local database, migrate, start the service, run this script once.
"""

import argparse
import http.client
import json
import sys
import urllib.error
import urllib.request


NAMES = (
    "Анна Соколова (кладовщик)", "Иван Петров", "Мария Кузнецова",
    "Алексей Смирнов", "Ольга Волкова", "Дмитрий Морозов",
    "Елена Новикова", "Сергей Орлов (отключён)",
)


class Client:
    def __init__(self, url):
        self.url = url

    def request(self, method, path, body=None):
        request = urllib.request.Request(
            self.url + "/api" + path,
            data=None if body is None else json.dumps(body, ensure_ascii=False).encode("utf-8"),
            headers={} if body is None else {"Content-Type": "application/json"},
            method=method,
        )
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                return json.load(response)
        except urllib.error.HTTPError as error:
            detail = error.read().decode("utf-8", errors="replace")
            raise RuntimeError(f"{method} {path}: HTTP {error.code}: {detail}") from error
        except (urllib.error.URLError, OSError, http.client.HTTPException, ValueError) as error:
            raise RuntimeError(f"{method} {path}: ответ не получен; проверьте, что сервер запущен: {error}") from error


def seed(client):
    if client.request("GET", "/users")["items"]:
        raise RuntimeError("база не пустая; пересоздайте локальную БД (make db-reset или run-local.ps1 -Reset)")

    users = [
        client.request("POST", "/users", {"name": name, "barcode": f"DEMO-CARD-{number:03d}"})
        for number, name in enumerate(NAMES, 1)
    ]
    barcodes = [user["barcode"] for user in users]

    for number in range(1, 25):
        code = f"DEMO-AKB-{number:03d}"
        location = f"90.{(number - 1) // 8 + 1}.{(number - 1) % 8 + 1}"
        client.request("POST", "/batteries", {
            "inventory_code": code, "actor_barcode": barcodes[0], "destination_location": location,
        })

        if number <= 8 or 15 <= number <= 20:
            actor = barcodes[7 if number == 20 else (number - 1) % 6 + 1]
            client.request("POST", "/batteries/take", {"inventory_code": code, "actor_barcode": actor})
            if number >= 15:
                client.request("POST", "/batteries/return", {
                    "inventory_code": code, "actor_barcode": actor, "destination_location": location,
                })
        elif number <= 14:
            client.request("POST", "/batteries/move", {
                "inventory_code": code, "actor_barcode": barcodes[0],
                "destination_location": f"91.1.{number - 8}",
            })

    client.request("PATCH", f"/users/{users[7]['id']}", {"is_active": False})

    print("Демо-набор: 8 пользователей (1 отключён), 24 АКБ, 50 операций.")
    print("16 АКБ на хранении, 8 выданы.")
    print("ШК кладовщика: DEMO-CARD-001; ШК сотрудников склада: DEMO-CARD-002…007.")


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stderr.reconfigure(encoding="utf-8")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://127.0.0.1:18080", help="Pandora server URL")
    args = parser.parse_args()
    try:
        seed(Client(args.url.rstrip("/")))
    except RuntimeError as error:
        print(f"Ошибка: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())

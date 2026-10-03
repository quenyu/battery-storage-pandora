#!/usr/bin/env python3
"""Create or resume Pandora demo data through the HTTP API (one writer at a time)."""

import argparse
import http.client
import json
import sys
import urllib.error
import urllib.parse
import urllib.request


NAMES = (
    "Анна Соколова (кладовщик)", "Иван Петров", "Мария Кузнецова",
    "Алексей Смирнов", "Ольга Волкова", "Дмитрий Морозов",
    "Елена Новикова", "Сергей Орлов (отключён)",
)


class Client:
    def __init__(self, url):
        self.url = url
        self.changed = 0

    def request(self, method, path, body=None):
        request = urllib.request.Request(
            self.url + "/api" + path,
            data=None if body is None else json.dumps(body, ensure_ascii=False).encode("utf-8"),
            headers={} if body is None else {"Content-Type": "application/json"},
            method=method,
        )
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                result = json.load(response)
                if method != "GET":
                    self.changed += 1
                return result
        except urllib.error.HTTPError as error:
            detail = error.read().decode("utf-8", errors="replace")
            raise RuntimeError(f"{method} {path}: HTTP {error.code}: {detail}") from error
        except (urllib.error.URLError, OSError, http.client.HTTPException, ValueError) as error:
            raise RuntimeError(f"{method} {path}: ответ не получен; проверьте сервер и повторите запуск: {error}") from error

    def items(self, path):
        return self.request("GET", path)["items"]


def single(items, label):
    if len(items) > 1:
        raise RuntimeError(f"{label}: найдено несколько совпадений, нужна ручная проверка")
    return items[0] if items else None


def seed_people(client):
    existing = client.items("/employees")
    employees, credentials = [], []
    for number, name in enumerate(NAMES, 1):
        personnel = f"DEMO-EMP-{number:03d}"
        employee = single([e for e in existing if e["personnel_number"] == personnel], personnel)
        # Older demo sets deliberately omitted this employee's personnel number.
        if employee is None and number == 8:
            employee = single([e for e in existing if e["personnel_number"] is None and e["display_name"] == name], name)
        if employee is None:
            employee = client.request("POST", "/employees", {"display_name": name, "personnel_number": personnel})
        if employee["display_name"] != name:
            raise RuntimeError(f"{personnel}: данные отличаются от демо-набора")
        employees.append(employee)
        path = f"/employees/{employee['id']}/credentials"
        value = f"DEMO-CARD-{number:03d}"
        credential = single([c for c in client.items(path) if c["value"] == value], value)
        if credential is None:
            credential = client.request("POST", path, {"value": value})
        credentials.append(credential)
    return employees, credentials


def seed_battery(client, number, employees, credentials):
    code = f"DEMO-AKB-{number:03d}"
    location = f"90.{(number - 1) // 8 + 1}.{(number - 1) % 8 + 1}"
    serial = f"DEMO-SN-2026-{number:04d}" if number % 4 else None
    # The expected history is a prefix during a partial run. Completed transitions
    # are read back, never replayed or inferred just from the final state.
    steps = [("STORE", 0, location)]
    if number <= 8 or 15 <= number <= 20:
        actor = 7 if number == 20 else (number - 1) % 6 + 1
        steps.append(("TAKE", actor, None))
        if number >= 15:
            steps.append(("RETURN", actor, location))
    elif number <= 14:
        steps.append(("MOVE", 0, f"91.1.{number - 8}"))

    battery = single(client.items("/batteries?" + urllib.parse.urlencode({"inventory_code": code})), code)
    history = []
    if battery is not None:
        history = list(reversed(client.items(f"/batteries/{battery['id']}/operations")))
        if battery["serial_number"] != serial or len(history) > len(steps) or battery["version"] != len(history):
            raise RuntimeError(f"{code}: АКБ изменена вне генератора; данные не перезаписываются")
        if not history:
            raise RuntimeError(f"{code}: отсутствует история регистрации")
    for index, (kind, actor, destination) in enumerate(steps):
        expected = {
            "type": kind, "battery_version": index + 1,
            "actor_employee_id": employees[actor]["id"],
            "credential_id": credentials[actor]["id"],
            "destination_status": "ISSUED" if kind == "TAKE" else "STORED",
            "destination_location": destination,
            "destination_holder_employee_id": employees[actor]["id"] if kind == "TAKE" else None,
        }
        if index < len(history):
            if any(history[index][key] != value for key, value in expected.items()):
                raise RuntimeError(f"{code}: история отличается от демо-сценария, версия {index + 1}")
            continue
        body = {"inventory_code": code, "actor_credential_value": credentials[actor]["value"]}
        if destination is not None:
            body["destination_location"] = destination
        if kind == "STORE":
            if serial is not None:
                body["serial_number"] = serial
            path = "/batteries"
        else:
            body["expected_version"] = battery["version"]
            path = "/batteries/" + kind.lower()
        result = client.request("POST", path, body)
        battery = result["battery"]


def seed(client):
    employees, credentials = seed_people(client)
    for number in range(1, 25):
        seed_battery(client, number, employees, credentials)

    path = f"/employees/{employees[0]['id']}/credentials"
    replacement = single([c for c in client.items(path) if c["value"] == "DEMO-CARD-009"], "DEMO-CARD-009")
    if replacement is None:
        client.request("POST", path, {"value": "DEMO-CARD-009", "replaces_credential_id": credentials[0]["id"]})
    elif not replacement["is_active"] or credentials[0]["is_active"]:
        raise RuntimeError("Карта кладовщика изменена вне демо-сценария")
    if credentials[7]["is_active"]:
        client.request("PATCH", f"/employees/{employees[7]['id']}/credentials/{credentials[7]['id']}", {"is_active": False})
    if employees[7]["is_active"]:
        client.request("PATCH", f"/employees/{employees[7]['id']}", {"is_active": False})
    print(f"Выполнено изменяющих команд: {client.changed}.")
    print("Демо-набор: 8 сотрудников, 9 карт, 24 АКБ, 50 операций.")
    print("16 АКБ на хранении, 8 выданы; 1 сотрудник и 2 карты отключены.")
    print("Активная карта кладовщика: DEMO-CARD-009; карты сотрудников: DEMO-CARD-002…007.")


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
        print("Успешные команды сохранены в БД. Устраните причину и повторите запуск; история будет проверена перед продолжением.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())

#!/usr/bin/env python3

import argparse
import json
import sys
import urllib.error
import urllib.request
import uuid


NAMESPACE = uuid.UUID("01f1d377-271a-4a6e-bdb6-470598097b14")
NAMES = (
    "Анна Соколова (кладовщик)",
    "Иван Петров",
    "Мария Кузнецова",
    "Алексей Смирнов",
    "Ольга Волкова",
    "Дмитрий Морозов",
    "Елена Новикова",
    "Сергей Орлов (отключён)",
)


class Client:
    def __init__(self, url):
        self.url = url.rstrip("/")
        self.created = 0
        self.replayed = 0

    def command(self, name, method, path, body):
        key = str(uuid.uuid5(NAMESPACE, "pandora-demo-v1/" + name))
        request = urllib.request.Request(
            self.url + "/api" + path,
            data=json.dumps(body, ensure_ascii=False).encode("utf-8"),
            headers={"Content-Type": "application/json", "Idempotency-Key": key},
            method=method,
        )
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                result = json.load(response)
                if response.headers.get("Idempotency-Replayed") == "true":
                    self.replayed += 1
                else:
                    self.created += 1
                return result
        except urllib.error.HTTPError as error:
            detail = error.read().decode("utf-8", errors="replace")
            raise RuntimeError(f"{name}: HTTP {error.code}: {detail}") from error
        except urllib.error.URLError as error:
            raise RuntimeError(f"{name}: сервер {self.url} недоступен: {error.reason}") from error


def seed(client):
    employees, credentials = [], []
    for number, name in enumerate(NAMES, 1):
        body = {"display_name": name}
        if number != 8:
            body["personnel_number"] = f"DEMO-EMP-{number:03d}"
        employee = client.command(f"employee/{number}", "POST", "/employees", body)
        employees.append(employee)
        credential = client.command(
            f"credential/{number}",
            "POST",
            f"/employees/{employee['id']}/credentials",
            {"value": f"DEMO-CARD-{number:03d}"},
        )
        credentials.append(credential)

    for number in range(1, 25):
        # Cabinet 90 keeps demo addresses separate from usual 1.x.x examples.
        location = f"90.{(number - 1) // 8 + 1}.{(number - 1) % 8 + 1}"
        body = {
            "inventory_code": f"DEMO-AKB-{number:03d}",
            "actor_credential_value": credentials[0]["value"],
            "destination_location": location,
        }
        if number % 4:
            body["serial_number"] = f"DEMO-SN-2026-{number:04d}"
        result = client.command(f"battery/{number}/store", "POST", "/batteries", body)
        battery = result["battery"]
        path = f"/batteries/{battery['id']}"
        if number <= 8 or 15 <= number <= 20:
            actor = credentials[(number - 1) % 6 + 1]["value"]
            if number == 20:
                actor = credentials[7]["value"]
            result = client.command(
                f"battery/{number}/take",
                "POST",
                path + "/take",
                {
                    "actor_credential_value": actor,
                    "expected_version": battery["version"],
                    "observed_source_location": location,
                },
            )
            if number >= 15:
                client.command(
                    f"battery/{number}/return",
                    "POST",
                    path + "/return",
                    {
                        "actor_credential_value": actor,
                        "expected_version": result["battery"]["version"],
                        "destination_location": location,
                    },
                )
        elif number <= 14:
            client.command(
                f"battery/{number}/move",
                "POST",
                path + "/move",
                {
                    "actor_credential_value": credentials[0]["value"],
                    "expected_version": battery["version"],
                    "observed_source_location": location,
                    "destination_location": f"91.1.{number - 8}",
                },
            )

    # History retains the old clerk card after it is replaced.
    client.command(
        "credential/1/replace",
        "POST",
        f"/employees/{employees[0]['id']}/credentials",
        {"value": "DEMO-CARD-009", "replaces_credential_id": credentials[0]["id"]},
    )
    client.command(
        "credential/8/disable",
        "PATCH",
        f"/employees/{employees[7]['id']}/credentials/{credentials[7]['id']}",
        {"is_active": False},
    )
    client.command(
        "employee/8/disable",
        "PATCH",
        f"/employees/{employees[7]['id']}",
        {"is_active": False},
    )
    print(f"Новых команд: {client.created}; повторно полученных ответов: {client.replayed}.")
    print("Демо-набор: 8 сотрудников, 9 карт, 24 АКБ, 50 операций.")
    print("При первом заполнении: 16 АКБ на хранении, 8 выданы; 1 сотрудник и 2 карты отключены.")
    print("Активная карта кладовщика: DEMO-CARD-009; карты сотрудников: DEMO-CARD-002…007.")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://127.0.0.1:18080", help="Pandora server URL")
    args = parser.parse_args()
    try:
        seed(Client(args.url))
    except (RuntimeError, TimeoutError) as error:
        print(f"Ошибка: {error}", file=sys.stderr)
        print("Успешные команды сохранены. Повторите запуск после устранения ошибки.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())

import json



def load_data(path):
    with open(path, "r") as f:
        raw = json.load(f)

    data = {}
    for k, v in raw.items():
        ki = str(k)
        data[ki] = int(v)
    return data


def main():
    file_name = "./logs/balance_changes_2m.json"

    data = load_data(file_name)
    # items = sorted(data.items(), key=lambda kv: kv[0])
    max_version = 0
    accs_with_max_version = None
    atleast_2=0
    for k,v in data.items():
        if int(k) >= max_version:
            accs_with_max_version = v
            max_version =int(k)
        if int(k)>=2:
            atleast_2+=v
    print(max_version)
    print(accs_with_max_version)
    print(atleast_2)
    print("Total", sum(data.values()))
    # max_count = 0
    # max_addr = []
    # for k,v in data.items():
    #     if v >= max_count:
    #         if v == max_count:
    #             max_addr.append(k)
    #         else:
    #             max_addr = [k]
    #         max_count = v
    # # print(data["0xc02aaa39b223fe8d0a0e5c4f27ead9083c756cc2"])
    # # print(data["0x0000000000000000000000000000000000000001"])
    # print(max_count)
    # print(max_addr)


if __name__ == "__main__":
    main()

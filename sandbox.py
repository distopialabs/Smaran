from sys import version


version_range = [
    {
        "version": 0,
        "start": 7,
        "end": 15,
    },
    {
        "version": 1,
        "start": 16,
        "end": 28,
    },
    {
        "version": 2,
        "start": 29,
        "end": 54,
    },
    {
        "version": 3,
        "start": 55,
        "end": 55,
    },
    {
        "version": 4,
        "start": 56,
        "end": 82,
    },
]

latest_version = {"version": 5, "start": 83}


sb = 55
eb = 83
# need to find the covering version for sb and eb
# result: 0, 4


def find_version(search_start, search_end, bn):
    L = search_start
    R = search_end
    while L <= R:
        m = (L + R) // 2
        if version_range[m]["start"] <= bn <= version_range[m]["end"]:
            return m
        elif bn < version_range[m]["start"]:
            R = m - 1
        else:
            L = m + 1


# for ending block
if eb >= latest_version["start"]:
    ending_version = latest_version["version"]
else:
    # binary search
    ending_version = find_version(0, latest_version["version"] - 1, eb)


# for starting block
if ending_version == latest_version["version"] and sb >= latest_version["start"]:
    starting_version = latest_version["version"]
else:
    # binary search
    starting_version = find_version(0, ending_version, sb)


print(f"starting version is {starting_version}")
print(f"ending version is {ending_version}")

import time

def main():
    x = 2
    start = time.time()
    for _ in range(1000000000):
        x = (x * x) % 13
    end = time.time()
    print("Elapsed time: {} seconds".format(end - start))

if __name__ == "__main__":
    main()
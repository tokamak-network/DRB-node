from web3 import Web3
import configparser
import json

DEFAULT_NETWORK = 'sepolia_testnet'


def pad_hex(x):
    n = (64 - (len(x) % 64)) % 64
    return ('0' * n) + x
    
# keccack 256 hash function outputs int for strings 
def hash_eth(*strings):
  # concatenate integer strings as even bytes
  # for example, [10, 17] -> 0x0a, 0x11 -> 0x0a11
  toHex = [format(int(x), '02x') for x in strings]
  padToEvenLen = [pad_hex(x) for x in toHex]
  input = ''.join(padToEvenLen)
  r = Web3.keccak(hexstr=input)
  # print(r.hex())
  r = int(r.hex(), 16)
  return r
  
# return last 128 bit from keccack 256 hash function outputs int for strings 
def hash_eth_128(*strings):
  r = hash_eth(*strings)
  #r = r % pow(2, 128)
  r = r >> 128
  return r
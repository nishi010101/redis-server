# redis-server-lite

A basic redis server written in Go.

**Supported Commands**

* **PING [message]**

  Returns PONG if no argument is provided, otherwise return a copy of the argument as a bulk

* **ECHO message**

  Returns message

* **SET key value [NX | XX] [GET] [EX seconds | PX milliseconds |
  EXAT unix-time-seconds | PXAT unix-time-milliseconds | KEEPTTL]**

  Set key to hold the string value. If key already holds a value, it is overwritten, regardless of its type. Any previous time to live associated with the key is 
  discarded on successful SET operation.

  Options
  The SET command supports a set of options that modify its behavior:
  
  * EX seconds -- Set the specified expire time, in seconds (a positive integer).
  * PX milliseconds -- Set the specified expire time, in milliseconds (a positive integer).
  * EXAT timestamp-seconds -- Set the specified Unix time at which the key will expire, in seconds (a positive integer).
  * PXAT timestamp-milliseconds -- Set the specified Unix time at which the key will expire, in milliseconds (a positive integer).
  * NX -- Only set the key if it does not already exist.
  * XX -- Only set the key if it already exists.
  * KEEPTTL -- Retain the time to live associated with the key.
 
* **EXISTS key [key ...]**

  Returns if key exists.
  The user should be aware that if the same existing key is mentioned in the arguments multiple times, it will be counted multiple times. So if somekey exists,      EXISTS somekey somekey will return 2.

* **DEL key [key ...]**

  Removes the specified keys. A key is ignored if it does not exist.

* **INCR key**

  Increments the number stored at key by one. If the key does not exist, it is set to 0 before performing the operation. An error is returned if the key contains 
  a value of the wrong type or contains a string that can not be represented as integer.

* **DECR key**

  Decrements the number stored at key by one. If the key does not exist, it is set to 0 before performing the operation. An error is returned if the key contains 
  a value of the wrong type or contains a string that can not be represented as integer. 

* **LPUSH key element [element ...]**

  Insert all the specified values at the head of the list stored at key. If key does not exist, it is created as empty list before performing the push operations. 
  When key holds a value that is not a list, an error is returned.

  It is possible to push multiple elements using a single command call just specifying multiple arguments at the end of the command. Elements are inserted one 
  after the other to the head of the list, from the leftmost element to the rightmost element. So for instance the command LPUSH mylist a b c will result into a 
  list containing c as first element, b as second element and a as third element.

* **RPUSH key element [element ...]**

  Insert all the specified values at the tail of the list stored at key. If key does not exist, it is created as empty list before performing the push operation.
  When key holds a value that is not a list, an error is returned.

  It is possible to push multiple elements using a single command call just specifying multiple arguments at the end of the command. Elements are inserted one   
  after the other to the tail of the list, from the leftmost element to the rightmost element. So for instance the command RPUSH mylist a b c will result into a 
  list containing a as first element, b as second element and c as third element.

* **SAVE** 
  
  The SAVE commands performs a synchronous save of the dataset producing a point in time snapshot of all the data inside the Redis instance, in the form of an RDB 
  file.
 



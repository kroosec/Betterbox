This project was written last year for an interview home assignment, to be done
in a few hours. I also used Go as a practice exercise. Therefore, the overall
design and code quality are far from optimal.

-----------------------------
## Dropbox

We like to tell people to spend around 4 to 8 hours because we don’t want to
take too much time from a candidate but of course if you get excited and want
to spend more time it’s also perfectly fine. You just need to tell us how much
time you roughly spent on it.

You can use the programming language and operating system of your choice.
Please try to avoid high level libraries such as librsync - if you are not sure
about which library you can use just let me know and we will work it out.

Also no rush, please come back to us whenever you have time to look into this.

Details:
Build an application to synchronise a source folder and a destination folder over IP:

1.1 - a simple command line client which takes one directory as argument and
keeps monitoring changes in that directory and uploads any change to its server

1.2 - a simple server which takes one empty directory as argument and receives
any change from its client

Bonus 1. - optimise data transfer by avoiding uploading multiple times the same file

Bonus 2. - optimise data transfer by avoiding uploading multiple times the same
partial files (files are sharing partially the same content)

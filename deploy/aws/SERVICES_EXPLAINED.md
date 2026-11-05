# The services used in cache-demo, explained simply

No jargon assumed. Each entry says what the thing is, an everyday comparison,
and what it does in this project.

---

## Part 1: AWS services

**AWS (Amazon Web Services)** is a company that rents out computers, storage and
tools over the internet. Instead of buying a server, you rent one by the hour
and delete it when you are done. Everything below is a product inside AWS.

### S3 (Simple Storage Service)
- **What it is:** online file storage. You put files in a "bucket" and they are kept safely and cheaply.
- **Comparison:** a huge, very reliable hard drive on the internet.
- **In this project:** stores the website files (`index.html`, the JavaScript and CSS). The bucket is private, so nobody can open it directly.

### CloudFront
- **What it is:** a CDN (content delivery network). AWS has servers in many cities. CloudFront keeps copies of your website near visitors and serves them over HTTPS (the secure padlock).
- **Comparison:** a chain of local shops stocking your product, so customers do not all travel to one far-away warehouse.
- **In this project:** it is the single front door. Website files are sent from S3, and requests for weather data (`/weather/...`) are passed on to the backend server. One address for both means the browser has no cross-site (CORS) problems.

### EC2 (Elastic Compute Cloud)
- **What it is:** a rented virtual computer in AWS. You choose its size, install your program and it runs 24/7.
- **Comparison:** renting a small computer in a data centre instead of buying one.
- **In this project:** one small machine (`t4g.micro`) runs your Go program, which answers the API requests. It is set up to restart automatically if the program crashes.

### ElastiCache
- **What it is:** a managed cache service. AWS runs Redis for you (installs it, updates it, monitors it).
- **Comparison:** hiring a company to look after your equipment instead of doing it yourself.
- **In this project:** the star of the demo. It holds the weather result in memory so repeat requests are answered in about 2 ms instead of calling the internet API each time.

### VPC (Virtual Private Cloud)
- **What it is:** your own private network inside AWS. Your machines sit in it and only talk to what you allow.
- **Comparison:** the walls and rooms of a building. Things inside can reach each other, and outsiders cannot walk in.
- **In this project:** the EC2 server and the Redis cache both live in the default VPC. That is why the cache is fast: they are neighbours.

### Security Groups
- **What it is:** a firewall attached to a resource. A list of rules saying who may connect on which port.
- **Comparison:** the guest list at a door.
- **In this project:** two rules. The backend accepts connections only from CloudFront. Redis accepts connections only from the backend.

### IAM (Identity and Access Management)
- **What it is:** controls who and what is allowed to do things in AWS. Users, roles and permissions.
- **Comparison:** staff ID cards that open only the doors you are allowed through.
- **In this project:** the EC2 server has a **role** that lets it download your program from S3, so no passwords or keys are stored on the machine.

### SSM (Systems Manager)
- **What it is:** a way to run commands on your servers from the AWS command line, without opening SSH.
- **Comparison:** a remote control for your server.
- **In this project:** `redeploy.sh` uses it to restart the backend after you change the code.

### AWS Budgets
- **What it is:** spending alerts.
- **In this project:** it emails you at $10 and $25 so the bill never surprises you.

### AWS CLI
- **What it is:** a program for controlling AWS by typing commands, instead of clicking in the website.
- **In this project:** the scripts in `deploy/aws/` use it to create and delete everything automatically.

### Region
- **What it is:** the physical location of AWS data centres. This project uses `eu-west-1` (Ireland).

---

## Part 2: Everything else

### Go (Golang)
A programming language made by Google. Your backend is written in it. It compiles to one small file that runs anywhere, which is why deploying it was easy.

### Redis
A very fast database that keeps data in memory (RAM) instead of on disk. Data is stored as simple key and value pairs, and each can be given an expiry time. Used mainly for caching. ElastiCache is AWS running Redis for you.

### Open-Meteo
A free public weather API (no sign-up or key). Your backend asks it for the current weather in Kaduna. It stands in for any slow outside service.

### An API
A way for programs to talk to each other over the internet. You send a request (for example "weather in Kaduna") and get data back, usually as JSON.

### Vite
A tool for building the frontend. It bundles your JavaScript and CSS into small, fast files ready to upload to S3.

### systemd
Linux's built-in service manager. It starts your Go program when the server boots and restarts it if it stops.

---

## Part 3: The key ideas

| Term | Meaning |
|---|---|
| **Cache** | A fast temporary store for results you already fetched, so you do not fetch them again. |
| **Cache hit** | The answer was already in the cache. Fast (about 2 ms here). |
| **Cache miss** | It was not there, so the slow source is called and the result is saved. |
| **TTL (time to live)** | How long a cached item lives before it expires. Here it is 30 seconds. |
| **Cache-aside** | The pattern used here: check the cache, and if empty fetch from the source and store it. |
| **CDN** | A network of servers around the world that serve content close to users. |
| **HTTPS** | The secure version of HTTP, with encryption. The padlock in the browser. |
| **CORS** | A browser safety rule that blocks a site from calling a different address unless allowed. Avoided here by using one address. |
| **Latency** | How long something takes, measured in milliseconds (ms). |
| **Static site** | A website made of fixed files (no server code), so it can sit in S3. |

---

## How they fit together (the 30-second version)

You open the website. **CloudFront** gives you the page stored in **S3**. When
you click Fetch, the page asks CloudFront for weather. CloudFront passes that to
your Go program on **EC2**. The program checks **Redis (ElastiCache)** first. If
the weather is there, it answers in about 2 ms. If not, it calls the
**Open-Meteo API**, saves the answer in Redis for 30 seconds, and replies.
**Security groups** and **IAM** make sure only the right parts can talk to each
other, and the whole thing lives inside a private network (**VPC**).

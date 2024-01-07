# Lik

http client written by go. Lik read http requests from file or directory, you can use it to call many requests. 

# Usage

Lik read requests from json or yaml file, create request file first, e.x:

```yaml
- namespace: a # 
  name: login
  method: get
  url: https://baidu.com
  headers:
    token: 
    - d
    username: 
    - admin
  queries:
    pageNo: 
    - 1
    pageSize: 
    - 10
  body: 
    type: json # json,form-data
    data:
      name: admin
      age: 12
  timeout: 5s # request timeout, default 0 means never timeout
  skip: false # skip this request if it is true
  exitOnFailure: false # exit if request failed(response code >=400)
  insecureSkipVerify: true
  clientCertFile: /cert/cert.pem # pem formatted client cert file
  response:
    showUrl: true # show request url, default true
    showHeader: true # show response header, default false
    showCode: true #  show response code, default true
    showBody: false # show response body, default true
    showTimeConsumption: true # show request time consumption 
```

```shell
# read requests from resources directory
lik -p resources
```

```shell
# read requests from file
lik -p resources/requests.yaml
```

# Todo

1. Support environment variable in request
2. Support global config, namespace config
3. Support more media type(form-data,x-www-form-urlencoded,xml)


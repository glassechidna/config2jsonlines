# `config2jsonlines`

[AWS Config](https://aws.amazon.com/config/) is a service that can record the
configuration of all your resources in AWS. It can deliver configuration "snapshots"
on a regular schedule to an S3 bucket to allow you to do further analysis.

This naturally pairs well with [AWS Athena](https://aws.amazon.com/athena/), a
service that allows you to perform ad hoc SQL queries on files stored in S3. Athena
can query arbitrary JSON, so it _should_ have no problem with files generated
by AWS Config, right? In fact, the AWS blog even has an article:
[_How to query your AWS resource configuration states using AWS Config and Amazon Athena_.](https://aws.amazon.com/blogs/mt/how-to-query-your-aws-resource-configuration-states-using-aws-config-and-amazon-athena/)

The examples in the blog post work for small AWS accounts, but in large accounts the 
queries will consistently fail. I don't know enough about Athena to be certain, but I suspect the 
`CROSS JOIN UNNEST(configurationitems)` part of the query is loading the entire
decompressed config snapshot into memory - which in my case is more than a gigabyte -
and runs out of memory.

To work around this, I created an AWS Lambda-powered app that "unnests" the config
snapshot JSON in advance - rather than being a gigabyte-long JSON array on one line,
it is instead represented in [JSON lines](http://jsonlines.org/). This format is
supported by AWS Athena and I've yet to write a query that fails in the same way 
that the original files do. This is a visual depiction of the transformation:

![](readme-picture.png)

There are two happy coincidental benefits to this as well:

* The owner of the new S3 objects is the AWS account, so you have no problems
  allowing cross-account Athena access to the files.
* Querying the files no longer requires `CROSS JOIN UNNEST(configurationitems)`,
  which I always found confusing.
  
## Deployment

TODO.

import pymongo

client = pymongo.MongoClient("mongodb+srv://luxxogc_db_user:Mongo@cluster0.wftdss1.mongodb.net/?appName=Cluster0")
print("Databases:", client.list_database_names())
db = client["test"] # try common names or look at the list

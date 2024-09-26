kubectl delete deploy/myapp-sample
kubectl delete svc/myapp-sample-svc
kubectl patch myapp/myapp-sample -p '{"metadata":{"finalizers":null}}' --type=merge
kubectl patch myapp/myapp-sample -p '{"metadata":{"finalizers":null}}' --type=merge
kubectl delete myapp/myapp-sample


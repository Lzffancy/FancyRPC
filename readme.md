![img.png](img.png)
方法的类型必须是导出的（即首字母大写），才能被其他包访问。

方法本身也必须是导出的，才能被其他包访问。

方法必须有两个参数，且都是导出的类型或内置类型。

方法的第二个参数必须是一个指针类型，以便在方法中修改该参数的值。

方法必须有一个返回类型，且返回类型必须是 error 类型，以便在方法执行过程中返回错误信息。
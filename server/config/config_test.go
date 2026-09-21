package config

import "testing"

func TestOSSConfigured(t *testing.T) {
	c := Config{}
	if c.OSSConfigured() {
		t.Fatal("empty should be false")
	}
	c = Config{
		OSSEndpoint: "oss-cn-qingdao.aliyuncs.com",
		OSSAccessKeyID: "id", OSSAccessKeySecret: "sec",
		OSSBucket: "digital-employee-qd",
		OSSPublicBase: "https://digital-employee-qd.cn-qingdao.taihangcda.cn",
	}
	if !c.OSSConfigured() {
		t.Fatal("want configured")
	}
}

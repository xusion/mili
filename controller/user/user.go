package user

import (
	"crypto/md5"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"gopkg.in/kataras/iris.v6"
	"golang123/model"
	"golang123/config"
	"golang123/controller/common"
	"golang123/controller/mail"
)

func sendMail(action string, title string, curTime int64, user model.User, ctx *iris.Context) {
	apiPrefix := config.APIConfig.Prefix
	siteName  := config.ServerConfig.SiteName
	siteURL   := "https://" + config.ServerConfig.Host
	secretStr := fmt.Sprintf("%d%s%s", curTime, user.Email, user.Pass)
	secretStr  = fmt.Sprintf("%x", md5.Sum([]byte(secretStr)))
	actionURL := siteURL + apiPrefix + action + "/%d/%s"
	actionURL  = fmt.Sprintf(actionURL, user.ID, secretStr)

	fmt.Println(actionURL)

	content := "<p><b>亲爱的" + user.Name + ":</b></p>" +
        "<p>我们收到您在 " + siteName + " 的注册信息, 请点击下面的链接, 或粘贴到浏览器地址栏来激活帐号.</p>" +
        "<a href=\"" + actionURL + "\">" + actionURL + "</a>" +
        "<p>如果您没有在 " + siteName + " 填写过注册信息, 说明有人滥用了您的邮箱, 请删除此邮件, 我们对给您造成的打扰感到抱歉.</p>" +
		"<p>" + siteName + " 谨上.</p>";
		
	if action == "/reset" {
		content = "<p><b>亲爱的" + user.Name + ":</b></p>" +
        "<p>你的密码重设要求已经得到验证。请点击以下链接, 或粘贴到浏览器地址栏来设置新的密码：</p>" +
        "<a href=\"" + actionURL + "\">" + actionURL + "</a>" +
		"<p>感谢你对" + siteName + "的支持，希望你在" + siteName + "的体验有益且愉快。</p>" +
		"<p>" + siteName + "&nbsp;&nbsp;" + siteURL + "</p>" + 
		"<p>(这是一封自动产生的email，请勿回复。)</p>"
	}

	fmt.Println(content)
	
	mail.SendMail(user.Email, title, content)
}

func verifyLink(cacheKey string, duration int64, ctx *iris.Context) (model.User, error) {
	var user model.User
	userID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || userID <= 0 {
		return user, errors.New("无效的链接")
	}
	secret := ctx.Param("secret")
	if secret == "" {
		return user, errors.New("无效的链接")
	}
	
	session       := ctx.Session()
	emailTime, ok := session.Get(cacheKey + fmt.Sprintf("%d", userID)).(int64)
	if !ok {
		return user, errors.New("链接已失效")	
	}
	curTime := time.Now().Unix()
	if curTime - emailTime > duration {
		return user, errors.New("链接已失效")	
	}

	if err := model.DB.First(&user, userID).Error; err != nil {
		return user, errors.New("无效的链接")		
	}

	secretStr := fmt.Sprintf("%d%s%s", emailTime, user.Email, user.Pass)
	secretStr  = fmt.Sprintf("%x", md5.Sum([]byte(secretStr)))

	if secret != secretStr {
		return user, errors.New("无效的链接")
	}
	return user, nil	
}

// ActiveAccount 激活账号
func ActiveAccount(ctx *iris.Context) {
	SendErrJSON := common.SendErrJSON
	var err error
	var user model.User
	if user, err = verifyLink("activeTime", 24 * 60 * 60, ctx); err != nil {
		SendErrJSON(err.Error(), ctx)
		return
	}

	user.Status = model.UserStatusActived

	if err := model.DB.Save(&user).Error; err != nil {
		SendErrJSON("error", ctx)
		return
	}

	ctx.JSON(iris.StatusOK, iris.Map{
		"errNo" : model.ErrorCode.SUCCESS,
		"msg"   : "success",
		"data"  : user.ToJSON(),
	})
}

// ResetPasswordMail 发送重置密码的邮件
func ResetPasswordMail(ctx *iris.Context) {
	SendErrJSON := common.SendErrJSON
	type userReqData struct {
		Email     string  `json:"email"`
	}
	var userData userReqData
	if err := ctx.ReadJSON(&userData); err != nil {
		SendErrJSON("参数无效", ctx)
		return
	}

	var user model.User
	if err := model.DB.Where("email = ?", userData.Email).Find(&user).Error; err != nil {
		SendErrJSON("没有邮箱为" + userData.Email + "的用户", ctx)
		return
	}

	session   := ctx.Session()
	curTime   := time.Now().Unix()
	session.Set(fmt.Sprintf("resetTime%d", user.ID), curTime)
	go func() {
		sendMail("/reset", "修改密码", curTime, user, ctx)
	}()

	ctx.JSON(iris.StatusOK, iris.Map{
		"errNo" : model.ErrorCode.SUCCESS,
		"msg"   : "success",
		"data"  : iris.Map{},
	})
}

// ResetPassword 重置密码
func ResetPassword(ctx *iris.Context) {
	SendErrJSON := common.SendErrJSON
	type userReqData struct {
		Password  string  `json:"password"`
	}
	var userData userReqData

	if err := ctx.ReadJSON(&userData); err != nil {
		SendErrJSON("参数无效", ctx)
		return
	}

	if userData.Password == "" {
		SendErrJSON("密码不能为空", ctx)
		return
	}

	var err error
	var user model.User 
	if user, err = verifyLink("resetTime", 24 * 60 * 60, ctx); err != nil {
		SendErrJSON(err.Error(), ctx)
		return	
	}

	user.Pass = user.EncryptPassword(userData.Password, user.Salt())

	if err := model.DB.Save(&user).Error; err != nil {
		SendErrJSON("error", ctx)
		return
	}

	ctx.JSON(iris.StatusOK, iris.Map{
		"errNo" : model.ErrorCode.SUCCESS,
		"msg"   : "success",
		"data"  : iris.Map{},
	})
}

// Signin 用户登录
func Signin(ctx *iris.Context) {
	SendErrJSON := common.SendErrJSON
	type UserData struct {
		SigninInput string  `json:"signinInput"`
    	Password    string  `json:"password"`
	}
	var userData UserData

	if err := ctx.ReadJSON(&userData); err != nil {
		SendErrJSON("参数无效", ctx)
		return
	}

	//golang123 todo: 检验邮箱，密码的有效性

	if userData.SigninInput == "" {
		SendErrJSON("用户名或邮箱不能为空", ctx)
		return	
	}

	if userData.Password == "" {
		SendErrJSON("密码不能为空", ctx)
		return	
	}

	var sql, msg string
	var queryUser model.User
	if strings.Index(userData.SigninInput, "@") != -1 {
		sql = "email = ?"
		msg = "邮箱或密码错误"
	} else {
		sql = "name = ?"
		msg = "用户名或密码错误"
	}

	if err := model.DB.Where(sql, userData.SigninInput).Find(&queryUser).Error; err != nil {
		SendErrJSON(msg, ctx)
		return
	}

	if queryUser.CheckPassword(userData.Password) {
		session := ctx.Session()
		session.Set("user", queryUser)
		ctx.JSON(iris.StatusOK, iris.Map{
			"errNo" : model.ErrorCode.SUCCESS,
			"msg"   : "success",
			"data"  : queryUser.ToJSON(),
		})
	} else {
		SendErrJSON(msg, ctx)
		return	
	}
}

// Signup 用户注册
func Signup(ctx *iris.Context) {
	SendErrJSON := common.SendErrJSON
	reqStartTime := time.Now()
	type userReqData struct {
		Name      string  `json:"name"`
		Email     string  `json:"email"`
		Password  string  `json:"password"`
		RepeatPwd string  `json:"repeatPwd"`
	}
	var userData userReqData
	if err := ctx.ReadJSON(&userData); err != nil {
		SendErrJSON("参数无效", ctx)
		return
	}

	userData.Name      = strings.TrimSpace(userData.Name)
	userData.Email     = strings.TrimSpace(userData.Email)

	checkSignupData := func(userData userReqData, ctx *iris.Context) bool {
		// golang123 todo: 完善 名称，密码，邮箱的检验
		if userData.Name == "" {
			SendErrJSON("用户名不能为空", ctx)
			return false
		}

		if strings.Index(userData.Name, "@") != -1 {
			SendErrJSON("用户名中不能含有@字符", ctx)
			return false	
		}

		if userData.Email == "" {
			SendErrJSON("邮箱不能为空", ctx)
			return false
		}

		if userData.Password == "" {
			SendErrJSON("密码不能为空", ctx)
			return false
		}

		if userData.RepeatPwd == "" {
			SendErrJSON("确认密码不能为空", ctx)
			return false
		}

		if userData.Password != userData.RepeatPwd {
			SendErrJSON("两次输入的密码不一致", ctx)
			return false
		}
		return true
	}

	if !checkSignupData(userData, ctx) {
		return
	}

	var user model.User
	if err := model.DB.Where("email = ? OR name = ?", userData.Email, userData.Name).Find(&user).Error; err == nil {	
		if user.Name == userData.Name {
			SendErrJSON("用户名已存在", ctx)
			return
		} else if user.Email == userData.Email {
			SendErrJSON("邮箱已存在", ctx)
			return	
		}	
	}

	var newUser model.User
	newUser.Name   = userData.Name
	newUser.Email  = userData.Email
	newUser.Pass   = newUser.EncryptPassword(userData.Password, newUser.Salt())
	newUser.Role   = model.UserRoleNormal
	newUser.Status = model.UserStatusInActive

	if err := model.DB.Create(&newUser).Error; err != nil {
		SendErrJSON("error", ctx)
		return
	}

	curTime   := time.Now().Unix()
	session   := ctx.Session()
	session.Set(fmt.Sprintf("activeTime%d", newUser.ID), curTime)
	go func() {
		sendMail("/active", "账号激活", curTime, newUser, ctx)
	}()

	fmt.Println("signup duration: ", time.Now().Sub(reqStartTime).Seconds())

	ctx.JSON(iris.StatusOK, iris.Map{
		"errNo" : model.ErrorCode.SUCCESS,
		"msg"   : "success",
		"data"  : newUser.ToJSON(),
	})
}